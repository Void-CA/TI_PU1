// Package httpapi exposes the prototype over HTTP with chi.
//
// Roles are simulated via the X-Role header ("admin" or "crew:{id}").
// This is NOT authentication or authorization: it only switches the two user
// experiences. Documented in the README.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/rules"
	"pu1/backend/internal/sim"
	"pu1/backend/internal/store"
)

type API struct {
	Store   *store.Store
	Weights rules.Weights
	Refs    rules.Refs
	Day     time.Time
}

func NewAPI(s *store.Store) *API {
	return &API{
		Store:   s,
		Weights: rules.DefaultWeights(),
		Refs:    rules.DefaultRefs(),
		Day:     store.OperatingDay,
	}
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", a.handleHealth)
		r.Get("/config", a.handleConfig)

		r.Get("/requests", a.handleListRequests)
		r.Post("/requests", a.handleCreateRequest)
		r.Post("/requests/{id}/evaluate", a.handleEvaluateRequest)

		r.Get("/orders", a.handleListOrders)
		r.Get("/orders/{id}", a.handleGetOrder)
		r.Post("/orders/{id}/candidates", a.handleCandidates)
		r.Post("/orders/{id}/status", a.handleOrderStatus)

		r.Get("/crews", a.handleListCrews)
		r.Get("/crews/{id}/schedule", a.handleSchedule)

		r.Post("/assignments/confirm", a.handleConfirm)
		r.Post("/assignments/{id}/reassign", a.handleReassign)
		r.Post("/assignments/{id}/cancel", a.handleCancel)

		r.Post("/plan", a.handlePlan)
		r.Post("/demo/concurrency", a.handleDemoConcurrency)
		r.Get("/evaluation", a.handleEvaluation)
	})
	return r
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Role")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (a *API) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrSlotTaken), errors.Is(err, store.ErrOrderNotPending):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrInvalidState):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrNoJustification):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// crewFromRole extracts the crew id from the simulated X-Role header.
// Returns nil when the role is not "crew:{id}" (admin or unset).
func crewFromRole(r *http.Request) *int64 {
	role := r.Header.Get("X-Role")
	var id int64
	if n, err := fmt.Sscanf(role, "crew:%d", &id); err == nil && n == 1 {
		return &id
	}
	return nil
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// --- handlers ---

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"operating_day": a.Day.Format("2006-01-02"),
		"weights":       a.Weights,
		"refs":          a.Refs,
		"service_types": []string{"fiber", "router", "splicing", "cabling", "coax", "fiber_splice"},
		"priorities": map[string]int{
			"low": int(domain.PriorityLow), "medium": int(domain.PriorityMedium), "high": int(domain.PriorityHigh),
		},
		"roles_note": "X-Role simulated: 'admin' or 'crew:{id}'. Not real authentication.",
	})
}

func (a *API) handleListRequests(w http.ResponseWriter, r *http.Request) {
	out, err := a.Store.ListRequests(r.Context())
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type createRequestDTO struct {
	Customer      string  `json:"customer"`
	ServiceType   string  `json:"service_type"`
	Description   string  `json:"description"`
	Priority      int     `json:"priority"`
	LocationX     float64 `json:"location_x"`
	LocationY     float64 `json:"location_y"`
	WindowStart   string  `json:"window_start"` // "HH:MM" on the operating day
	WindowEnd     string  `json:"window_end"`
	DurationMin   int     `json:"duration_min"`
}

func (a *API) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	var dto createRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if dto.Customer == "" || dto.ServiceType == "" || dto.DurationMin <= 0 ||
		dto.Priority < 1 || dto.Priority > 3 {
		writeError(w, http.StatusBadRequest, "customer, service_type, priority (1..3) and duration_min are required")
		return
	}
	start, ok1 := a.parseHHMM(dto.WindowStart)
	end, ok2 := a.parseHHMM(dto.WindowEnd)
	if !ok1 || !ok2 || !end.After(start) {
		writeError(w, http.StatusBadRequest, "window_start/window_end must be HH:MM with end after start")
		return
	}
	req, err := a.Store.CreateRequest(r.Context(), domain.Request{
		Customer: dto.Customer, ServiceType: dto.ServiceType, Description: dto.Description,
		Priority: domain.Priority(dto.Priority), LocationX: dto.LocationX, LocationY: dto.LocationY,
		WindowStart: start, WindowEnd: end, DurationMin: dto.DurationMin,
	})
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (a *API) parseHHMM(s string) (time.Time, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(a.Day.Year(), a.Day.Month(), a.Day.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC), true
}

func (a *API) handleEvaluateRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var dto struct {
		Remote bool `json:"remote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	rv, err := a.Store.EvaluateRequest(r.Context(), id, dto.Remote)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rv)
}

func (a *API) handleListOrders(w http.ResponseWriter, r *http.Request) {
	var status *domain.OrderStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := domain.OrderStatus(s)
		status = &st
	}
	out, err := a.Store.ListOrders(r.Context(), status)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	order, err := a.Store.GetOrder(r.Context(), id)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a *API) handleCandidates(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	cands, err := a.Store.Candidates(r.Context(), id, a.Day, a.Weights, a.Refs)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cands)
}

type orderStatusDTO struct {
	Status string `json:"status"`
}

func (a *API) handleOrderStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var dto orderStatusDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	order, err := a.Store.UpdateOrderStatus(r.Context(), id, domain.OrderStatus(dto.Status), crewFromRole(r))
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a *API) handleListCrews(w http.ResponseWriter, r *http.Request) {
	out, err := a.Store.ListCrews(r.Context(), a.Day)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	out, err := a.Store.Schedule(r.Context(), id, a.Day)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type confirmDTO struct {
	OrderID       int64  `json:"order_id"`
	CrewID        int64  `json:"crew_id"`
	Justification string `json:"justification"`
}

func (a *API) handleConfirm(w http.ResponseWriter, r *http.Request) {
	var dto confirmDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if dto.OrderID == 0 || dto.CrewID == 0 {
		writeError(w, http.StatusBadRequest, "order_id and crew_id are required")
		return
	}
	as, err := a.Store.Confirm(r.Context(), store.ConfirmInput{
		OrderID: dto.OrderID, CrewID: dto.CrewID, Justification: dto.Justification,
	}, a.Day, a.Weights, a.Refs)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, as)
}

type reassignDTO struct {
	CrewID        int64  `json:"crew_id"`
	Justification string `json:"justification"`
}

func (a *API) handleReassign(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var dto reassignDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if dto.CrewID == 0 {
		writeError(w, http.StatusBadRequest, "crew_id is required")
		return
	}
	as, err := a.Store.Reassign(r.Context(), id, dto.CrewID, dto.Justification, a.Day)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, as)
}

func (a *API) handleCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var dto struct {
		Justification string `json:"justification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := a.Store.CancelAssignment(r.Context(), id, dto.Justification); err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (a *API) handlePlan(w http.ResponseWriter, r *http.Request) {
	results, err := a.Store.PlanDay(r.Context(), a.Day, a.Weights, a.Refs)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	assigned := 0
	for _, res := range results {
		if res.Assigned {
			assigned++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orders_total":   len(results),
		"orders_assigned": assigned,
		"results":        results,
	})
}

func (a *API) handleDemoConcurrency(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		CrewID   int64 `json:"crew_id"`
		Attempts int   `json:"attempts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if dto.CrewID == 0 {
		dto.CrewID = 3
	}
	if dto.Attempts == 0 {
		dto.Attempts = 3
	}
	res, err := a.Store.DemoConcurrency(r.Context(), dto.CrewID, a.Day, dto.Attempts)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) handleEvaluation(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, sim.Run())
}
