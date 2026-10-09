package assignments

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"pu1/backend/internal/platform/httplib"
)

type confirmDTO struct {
	OrderID       int64  `json:"order_id"`
	CrewID        int64  `json:"crew_id"`
	Justification string `json:"justification"`
}

type reassignDTO struct {
	CrewID        int64  `json:"crew_id"`
	Justification string `json:"justification"`
}

type justificationDTO struct {
	Justification string `json:"justification"`
}

type demoDTO struct {
	CrewID   int64 `json:"crew_id"`
	Attempts int   `json:"attempts"`
}

// Routes mounts the assignment routes on r using the shared service.
func (s *Service) Routes(r chi.Router) {
	r.Post("/orders/{id}/candidates", s.handleCandidates)
	r.Post("/assignments/confirm", s.handleConfirm)
	r.Post("/assignments/{id}/reassign", s.handleReassign)
	r.Post("/assignments/{id}/cancel", s.handleCancel)
	r.Post("/demo/concurrency", s.handleDemo)
}

func (s *Service) handleCandidates(w http.ResponseWriter, req *http.Request) {
	id, ok := httplib.IDParam(w, req)
	if !ok {
		return
	}
	cands, err := s.Candidates(req.Context(), id)
	if err != nil {
		httplib.StoreError(w, err)
		return
	}
	httplib.WriteJSON(w, http.StatusOK, cands)
}

func (s *Service) handleConfirm(w http.ResponseWriter, req *http.Request) {
	var dto confirmDTO
	if err := json.NewDecoder(req.Body).Decode(&dto); err != nil {
		httplib.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if dto.OrderID == 0 || dto.CrewID == 0 {
		httplib.WriteError(w, http.StatusBadRequest, "order_id and crew_id are required")
		return
	}
	as, err := s.Confirm(req.Context(), ConfirmInput{
		OrderID: dto.OrderID, CrewID: dto.CrewID, Justification: dto.Justification,
	})
	if err != nil {
		httplib.StoreError(w, err)
		return
	}
	httplib.WriteJSON(w, http.StatusCreated, as)
}

func (s *Service) handleReassign(w http.ResponseWriter, req *http.Request) {
	id, ok := httplib.IDParam(w, req)
	if !ok {
		return
	}
	var dto reassignDTO
	if err := json.NewDecoder(req.Body).Decode(&dto); err != nil {
		httplib.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if dto.CrewID == 0 {
		httplib.WriteError(w, http.StatusBadRequest, "crew_id is required")
		return
	}
	as, err := s.Reassign(req.Context(), id, dto.CrewID, dto.Justification)
	if err != nil {
		httplib.StoreError(w, err)
		return
	}
	httplib.WriteJSON(w, http.StatusOK, as)
}

func (s *Service) handleCancel(w http.ResponseWriter, req *http.Request) {
	id, ok := httplib.IDParam(w, req)
	if !ok {
		return
	}
	var dto justificationDTO
	if err := json.NewDecoder(req.Body).Decode(&dto); err != nil {
		httplib.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.Cancel(req.Context(), id, dto.Justification); err != nil {
		httplib.StoreError(w, err)
		return
	}
	httplib.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Service) handleDemo(w http.ResponseWriter, req *http.Request) {
	var dto demoDTO
	if err := json.NewDecoder(req.Body).Decode(&dto); err != nil && !errors.Is(err, io.EOF) {
		httplib.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if dto.CrewID == 0 {
		dto.CrewID = 3
	}
	if dto.Attempts == 0 {
		dto.Attempts = 3
	}
	res, err := s.DemoConcurrency(req.Context(), dto.CrewID, dto.Attempts)
	if err != nil {
		httplib.StoreError(w, err)
		return
	}
	httplib.WriteJSON(w, http.StatusOK, res)
}
