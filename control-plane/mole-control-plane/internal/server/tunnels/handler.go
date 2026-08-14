package tunnels

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"mole-control-plane/internal/server/auth"
	"mole-control-plane/internal/server/httpx"
	"mole-control-plane/internal/tunnel"
)

type createTunnelRequest struct {
	Protocol        string `json:"proto"`
	InternalAddress string `json:"internal_address"`
}

type connectTunnelRequest struct {
	Token string `json:"token"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.SetupErr != nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel provisioning is not configured"})
		return
	}
	account := auth.UserFromContext(r.Context())

	var request createTunnelRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		return
	}
	created, err := h.Service.Create(r.Context(), account.ID, tunnel.CreateInput{
		Protocol:        request.Protocol,
		InternalAddress: request.InternalAddress,
	})
	if err != nil {
		switch {
		case errors.Is(err, tunnel.ErrInvalidInput):
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "proto and internal_address are invalid"})
		case errors.Is(err, tunnel.ErrLimitReached):
			httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": "plan limit reached"})
		case errors.Is(err, tunnel.ErrUnavailable):
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel provisioning is not configured"})
		default:
			httpx.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "unable to provision tunnel"})
		}
		return
	}
	h.NotifyUserUpdate(r.Context(), account.ID)
	httpx.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.SetupErr != nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel provisioning is not configured"})
		return
	}
	account := auth.UserFromContext(r.Context())
	tunnelID := chi.URLParam(r, "tunnelID")
	err := h.Service.Delete(r.Context(), account.ID, tunnelID)
	if err != nil {
		switch {
		case errors.Is(err, tunnel.ErrInvalidInput):
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "tunnel id is required"})
		case errors.Is(err, tunnel.ErrNotFound):
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel not found"})
		case errors.Is(err, tunnel.ErrUnavailable):
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel provisioning is not configured"})
		default:
			httpx.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "unable to delete tunnel"})
		}
		return
	}
	h.NotifyUserUpdate(r.Context(), account.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	var request connectTunnelRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		return
	}
	config, err := h.Service.ConnectionConfigForToken(r.Context(), request.Token)
	if errors.Is(err, tunnel.ErrNotFound) {
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid tunnel token"})
		return
	}
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to connect tunnel"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, config)
}
