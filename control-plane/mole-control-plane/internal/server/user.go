package server

import (
	"errors"
	"net/http"

	"mole-control-plane/internal/user"
)

func (s *Server) listPlansHandler(w http.ResponseWriter, r *http.Request) {
	plans, err := s.users.ListPlans(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to list plans"})
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func (s *Server) currentUserHandler(w http.ResponseWriter, r *http.Request) {
	account := userFromContext(r.Context())

	profile, err := s.users.Profile(r.Context(), account.ID)
	if err != nil {
		if errors.Is(err, user.ErrUnauthenticated) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to load user profile"})
		return
	}
	writeJSON(w, http.StatusOK, profile)
}
