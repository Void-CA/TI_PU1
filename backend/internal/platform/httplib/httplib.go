// Package httplib holds the small HTTP helpers shared by every feature
// handler: JSON responses, error mapping, CORS and the simulated role header.
package httplib

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pu1/backend/internal/platform/db"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// StoreError maps the shared sentinel errors to HTTP status codes.
func StoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, db.ErrSlotTaken), errors.Is(err, db.ErrOrderNotPending):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, db.ErrInvalidState):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, db.ErrNoJustification):
		WriteError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("internal error: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

// CrewFromRole extracts the crew id from the simulated X-Role header.
// Returns nil when the role is not "crew:{id}" (admin or unset).
// NOT real authentication: only switches the two user experiences.
func CrewFromRole(r *http.Request) *int64 {
	role := r.Header.Get("X-Role")
	var id int64
	if n, err := fmt.Sscanf(role, "crew:%d", &id); err == nil && n == 1 {
		return &id
	}
	return nil
}

// IDParam parses the {id} URL parameter.
func IDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// CORS allows the Vite dev server to reach the API directly.
func CORS(next http.Handler) http.Handler {
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
