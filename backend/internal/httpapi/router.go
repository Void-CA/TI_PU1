// Package httpapi assembles the application: it mounts every feature's routes
// on a single chi router. It contains no business logic and no SQL.
package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/engine"
	"pu1/backend/internal/features/assignments"
	"pu1/backend/internal/features/crews"
	"pu1/backend/internal/features/evaluation"
	"pu1/backend/internal/features/orders"
	"pu1/backend/internal/features/planning"
	"pu1/backend/internal/features/requests"
	"pu1/backend/internal/platform/httplib"
)

// Router assembles all feature routes. Weights/refs are the engine parameters
// shared by the assignment endpoints.
func Router(pool *pgxpool.Pool, day time.Time, w engine.Weights, r engine.Refs) http.Handler {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)
	mux.Use(httplib.CORS)

	// Shared assignment service: used by its own routes and by planning.
	asvc := assignments.NewService(pool, day, w, r)
	osvc := orders.NewRepo(pool)
	psvc := planning.NewService(osvc, asvc, day)

	mux.Route("/api", func(api chi.Router) {
		api.Get("/health", func(w http.ResponseWriter, req *http.Request) {
			httplib.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})
		api.Get("/config", func(w http.ResponseWriter, req *http.Request) {
			httplib.WriteJSON(w, http.StatusOK, map[string]any{
				"operating_day": day.Format("2006-01-02"),
				"weights":       w,
				"refs":          r,
				"service_types": []string{"fiber", "router", "splicing", "cabling", "coax", "fiber_splice"},
				"priorities": map[string]int{
					"low": int(domain.PriorityLow), "medium": int(domain.PriorityMedium), "high": int(domain.PriorityHigh),
				},
				"roles_note": "X-Role simulated: 'admin' or 'crew:{id}'. Not real authentication.",
			})
		})

		requests.Register(api, pool, day)
		orders.Register(api, pool, day)
		crews.Register(api, pool, day)
		asvc.Routes(api)
		psvc.Routes(api)
		evaluation.Routes(api)
	})
	return mux
}
