package users

import (
	"errors"
	"net/http"

	"mole-control-plane/internal/server/auth"
	"mole-control-plane/internal/server/httpx"
	"mole-control-plane/internal/user"
)

func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.Users.ListPlans(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to list plans"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, plans)
}

func (h *Handler) CurrentUser(w http.ResponseWriter, r *http.Request) {
	account := auth.UserFromContext(r.Context())

	if err := h.Tunnels.RefreshUserUsage(r.Context(), account.ID); err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to refresh user usage"})
		return
	}

	profile, err := h.Users.Profile(r.Context(), account.ID)
	if err != nil {
		if errors.Is(err, user.ErrUnauthenticated) {
			httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to load user profile"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, profile)
}
