package orders

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/platform/httplib"
)

type statusDTO struct {
	Status string `json:"status"`
}

// Register mounts the orders routes on r.
func Register(r chi.Router, pool *pgxpool.Pool, _ time.Time) {
	repo := NewRepo(pool)

	r.Get("/orders", func(w http.ResponseWriter, req *http.Request) {
		var status *domain.OrderStatus
		if s := req.URL.Query().Get("status"); s != "" {
			st := domain.OrderStatus(s)
			status = &st
		}
		out, err := repo.ListOrders(req.Context(), status)
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		httplib.WriteJSON(w, http.StatusOK, out)
	})

	r.Get("/orders/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, ok := httplib.IDParam(w, req)
		if !ok {
			return
		}
		order, err := repo.GetOrder(req.Context(), id)
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		httplib.WriteJSON(w, http.StatusOK, order)
	})

	r.Post("/orders/{id}/status", func(w http.ResponseWriter, req *http.Request) {
		id, ok := httplib.IDParam(w, req)
		if !ok {
			return
		}
		var dto statusDTO
		if err := json.NewDecoder(req.Body).Decode(&dto); err != nil {
			httplib.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		order, err := repo.UpdateStatus(req.Context(), id, domain.OrderStatus(dto.Status), httplib.CrewFromRole(req))
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		httplib.WriteJSON(w, http.StatusOK, order)
	})
}
