package requests

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/platform/httplib"
)

type createDTO struct {
	Customer    string  `json:"customer"`
	ServiceType string  `json:"service_type"`
	Description string  `json:"description"`
	Priority    int     `json:"priority"`
	LocationX   float64 `json:"location_x"`
	LocationY   float64 `json:"location_y"`
	WindowStart string  `json:"window_start"` // "HH:MM" on the operating day
	WindowEnd   string  `json:"window_end"`
	DurationMin int     `json:"duration_min"`
}

type evaluateDTO struct {
	Remote bool `json:"remote"`
}

// Register mounts the requests routes on r.
func Register(r chi.Router, pool *pgxpool.Pool, day time.Time) {
	repo := NewRepo(pool)

	parseHHMM := func(s string) (time.Time, bool) {
		t, err := time.Parse("15:04", s)
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC), true
	}

	r.Get("/requests", func(w http.ResponseWriter, req *http.Request) {
		out, err := repo.ListRequests(req.Context())
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		httplib.WriteJSON(w, http.StatusOK, out)
	})

	r.Post("/requests", func(w http.ResponseWriter, req *http.Request) {
		var dto createDTO
		if err := json.NewDecoder(req.Body).Decode(&dto); err != nil {
			httplib.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if dto.Customer == "" || dto.ServiceType == "" || dto.DurationMin <= 0 ||
			dto.Priority < 1 || dto.Priority > 3 {
			httplib.WriteError(w, http.StatusBadRequest, "customer, service_type, priority (1..3) and duration_min are required")
			return
		}
		start, ok1 := parseHHMM(dto.WindowStart)
		end, ok2 := parseHHMM(dto.WindowEnd)
		if !ok1 || !ok2 || !end.After(start) {
			httplib.WriteError(w, http.StatusBadRequest, "window_start/window_end must be HH:MM with end after start")
			return
		}
		created, err := repo.CreateRequest(req.Context(), domain.Request{
			Customer: dto.Customer, ServiceType: dto.ServiceType, Description: dto.Description,
			Priority: domain.Priority(dto.Priority), LocationX: dto.LocationX, LocationY: dto.LocationY,
			WindowStart: start, WindowEnd: end, DurationMin: dto.DurationMin,
		})
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		httplib.WriteJSON(w, http.StatusCreated, created)
	})

	r.Post("/requests/{id}/evaluate", func(w http.ResponseWriter, req *http.Request) {
		id, ok := httplib.IDParam(w, req)
		if !ok {
			return
		}
		var dto evaluateDTO
		if err := json.NewDecoder(req.Body).Decode(&dto); err != nil {
			httplib.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		rv, err := repo.EvaluateRequest(req.Context(), id, dto.Remote)
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		httplib.WriteJSON(w, http.StatusOK, rv)
	})
}
