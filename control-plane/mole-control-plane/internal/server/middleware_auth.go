package server

import (
	"context"
	"net/http"

	"mole-control-plane/internal/user"
)

type authenticatedUserContextKey struct{}

// requireAuthentication authenticates the request and makes the account
// available to handlers through the request context.
func (s *Server) requireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.users == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}

		account, err := s.users.Authenticate(r.Context(), bearerToken(r))
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}

		ctx := context.WithValue(r.Context(), authenticatedUserContextKey{}, account)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAdministrator permits only authenticated users with administrator
// privileges. It must be applied after requireAuthentication.
func requireAdministrator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !userFromContext(r.Context()).IsAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access required"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func userFromContext(ctx context.Context) user.User {
	account, _ := ctx.Value(authenticatedUserContextKey{}).(user.User)
	return account
}
