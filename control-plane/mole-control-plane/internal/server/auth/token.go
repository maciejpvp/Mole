package auth

import (
	"net/http"
	"strings"
)

// BearerToken returns an access token from the Authorization header or token query parameter.
func BearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if strings.HasPrefix(value, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}
