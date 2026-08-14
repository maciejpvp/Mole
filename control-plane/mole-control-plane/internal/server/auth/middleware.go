package auth

import (
	"context"
	"net/http"

	"mole-control-plane/internal/server/httpx"
	"mole-control-plane/internal/user"
)

type authenticatedUserContextKey struct{}

// requireAuthentication authenticates the request and makes the account
// available to handlers through the request context.
func RequireAuthentication(users *user.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if users == nil {
				httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
				return
			}

			account, err := users.Authenticate(r.Context(), BearerToken(r))
			if err != nil {
				httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
				return
			}

			ctx := context.WithValue(r.Context(), authenticatedUserContextKey{}, account)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// requireAdministrator permits only authenticated users with administrator
// privileges. It must be applied after requireAuthentication.
func RequireAdministrator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !UserFromContext(r.Context()).IsAdmin {
			httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access required"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func UserFromContext(ctx context.Context) user.User {
	account, _ := ctx.Value(authenticatedUserContextKey{}).(user.User)
	return account
}
