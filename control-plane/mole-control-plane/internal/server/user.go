package server

import (
	"errors"
	"net/http"

	"mole-control-plane/internal/user"
)

func (s *Server) currentUserHandler(w http.ResponseWriter, r *http.Request) {
	account, err := s.authenticatedUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

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
