package crews

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/platform/httplib"
)

// Register mounts the crews routes on r.
func Register(r chi.Router, pool *pgxpool.Pool, day time.Time) {
	repo := NewRepo(pool)

	r.Get("/crews", func(w http.ResponseWriter, req *http.Request) {
		out, err := repo.ListCrews(req.Context(), day)
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		httplib.WriteJSON(w, http.StatusOK, out)
	})

	r.Get("/crews/{id}/schedule", func(w http.ResponseWriter, req *http.Request) {
		id, ok := httplib.IDParam(w, req)
		if !ok {
			return
		}
		out, err := repo.Schedule(req.Context(), id, day)
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		httplib.WriteJSON(w, http.StatusOK, out)
	})
}
