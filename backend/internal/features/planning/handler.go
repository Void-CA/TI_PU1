package planning

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"pu1/backend/internal/platform/httplib"
)

// Routes mounts the planning routes on r.
func (s *Service) Routes(r chi.Router) {
	r.Post("/plan", func(w http.ResponseWriter, req *http.Request) {
		results, err := s.PlanDay(req.Context())
		if err != nil {
			httplib.StoreError(w, err)
			return
		}
		assigned := 0
		for _, res := range results {
			if res.Assigned {
				assigned++
			}
		}
		httplib.WriteJSON(w, http.StatusOK, map[string]any{
			"orders_total":    len(results),
			"orders_assigned": assigned,
			"results":         results,
		})
	})
}
