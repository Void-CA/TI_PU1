package evaluation

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"pu1/backend/internal/platform/httplib"
)

// Routes mounts the evaluation routes on r.
func Routes(r chi.Router) {
	r.Get("/evaluation", func(w http.ResponseWriter, req *http.Request) {
		httplib.WriteJSON(w, http.StatusOK, Run())
	})
}
