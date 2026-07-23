package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"mole-control-plane/internal/tunnel"
	"mole-control-plane/internal/user"
)

type createTunnelRequest struct {
	Protocol        string `json:"proto"`
	InternalAddress string `json:"internal_address"`
}

type usageSyncRequest struct {
	Updates []tunnel.UsageUpdate `json:"updates"`
}

type connectTunnelRequest struct {
	Token string `json:"token"`
}

type connectionStatusRequest struct {
	TunnelID string `json:"tunnel_id"`
	Status   string `json:"status"`
}

func (s *Server) createTunnelHandler(w http.ResponseWriter, r *http.Request) {
	if s.tunnelSetupErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel provisioning is not configured"})
		return
	}
	account, err := s.authenticatedUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	var request createTunnelRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	created, err := s.tunnels.Create(r.Context(), account.ID, tunnel.CreateInput{
		Protocol:        request.Protocol,
		InternalAddress: request.InternalAddress,
	})
	if err != nil {
		switch {
		case errors.Is(err, tunnel.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "proto and internal_address are invalid"})
		case errors.Is(err, tunnel.ErrLimitReached):
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "plan limit reached"})
		case errors.Is(err, tunnel.ErrUnavailable):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel provisioning is not configured"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "unable to provision tunnel"})
		}
		return
	}
	s.notifyUserUpdate(r.Context(), account.ID)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) deleteTunnelHandler(w http.ResponseWriter, r *http.Request) {
	if s.tunnelSetupErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel provisioning is not configured"})
		return
	}
	account, err := s.authenticatedUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	tunnelID := chi.URLParam(r, "tunnelID")
	err = s.tunnels.Delete(r.Context(), account.ID, tunnelID)
	if err != nil {
		switch {
		case errors.Is(err, tunnel.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tunnel id is required"})
		case errors.Is(err, tunnel.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel not found"})
		case errors.Is(err, tunnel.ErrUnavailable):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel provisioning is not configured"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "unable to delete tunnel"})
		}
		return
	}
	s.notifyUserUpdate(r.Context(), account.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncTunnelUsageHandler(w http.ResponseWriter, r *http.Request) {
	sharedToken := os.Getenv("TUNNEL_SERVER_API_TOKEN")
	if sharedToken == "" || subtle.ConstantTimeCompare([]byte(sharedToken), []byte(bearerToken(r))) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var request usageSyncRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid usage payload"})
		return
	}
	response, err := s.tunnels.ApplyUsage(r.Context(), request.Updates)
	if err != nil {
		if errors.Is(err, tunnel.ErrInvalidInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid usage payload"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to sync usage"})
		return
	}
	s.notifyUserUpdateForUsage(r.Context(), request.Updates)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) syncTunnelConnectionStatusHandler(w http.ResponseWriter, r *http.Request) {
	sharedToken := os.Getenv("TUNNEL_SERVER_API_TOKEN")
	if sharedToken == "" || subtle.ConstantTimeCompare([]byte(sharedToken), []byte(bearerToken(r))) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var request connectionStatusRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	if err := s.tunnels.SetConnectionStatus(r.Context(), request.TunnelID, request.Status); err != nil {
		switch {
		case errors.Is(err, tunnel.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid connection status"})
		case errors.Is(err, tunnel.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel not found"})
		case errors.Is(err, tunnel.ErrLimitReached):
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "plan limit reached"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to update tunnel status"})
		}
		return
	}
	s.notifyUserUpdateForTunnel(r.Context(), request.TunnelID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) connectTunnelHandler(w http.ResponseWriter, r *http.Request) {
	var request connectTunnelRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	config, err := s.tunnels.ConnectionConfigForToken(r.Context(), request.Token)
	if errors.Is(err, tunnel.ErrNotFound) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid tunnel token"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to connect tunnel"})
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) authenticatedUser(r *http.Request) (user.User, error) {
	if s.users == nil {
		return user.User{}, user.ErrUnauthenticated
	}
	return s.users.Authenticate(r.Context(), bearerToken(r))
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if strings.HasPrefix(value, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
	}
	if queryToken := r.URL.Query().Get("token"); queryToken != "" {
		return strings.TrimSpace(queryToken)
	}
	return ""
}
