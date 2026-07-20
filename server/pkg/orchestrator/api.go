package orchestrator

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// ManagementAPI exposes the authenticated control-plane-only provisioning API.
type ManagementAPI struct {
	engine *Engine
	token  string
}

func NewManagementAPI(engine *Engine, token string) *ManagementAPI {
	return &ManagementAPI{engine: engine, token: token}
}

func (a *ManagementAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/tunnels":
		a.provision(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/tunnels/"):
		tunnelID := strings.TrimPrefix(r.URL.Path, "/v1/tunnels/")
		if tunnelID == "" || strings.Contains(tunnelID, "/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		if err := a.engine.Deprovision(tunnelID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to deprovision tunnel"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (a *ManagementAPI) provision(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		TunnelID                  string `json:"tunnel_id"`
		UserID                    string `json:"user_id"`
		Protocol                  string `json:"protocol"`
		Token                     string `json:"token"`
		MonthlyMinutesLimit       *int64 `json:"monthly_minutes_limit"`
		MonthlyTransferBytesLimit *int64 `json:"monthly_transfer_bytes_limit"`
		MonthlyMinutesUsed        int64  `json:"monthly_minutes_used"`
		MonthlyTransferBytesUsed  int64  `json:"monthly_transfer_bytes_used"`
	}
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provision request"})
		return
	}
	response, err := a.engine.Provision(ProvisionRequest{
		TunnelID:                  request.TunnelID,
		UserID:                    request.UserID,
		Protocol:                  request.Protocol,
		Token:                     request.Token,
		MonthlyMinutesLimit:       request.MonthlyMinutesLimit,
		MonthlyTransferBytesLimit: request.MonthlyTransferBytesLimit,
		MonthlyMinutesUsed:        request.MonthlyMinutesUsed,
		MonthlyTransferBytesUsed:  request.MonthlyTransferBytesUsed,
	})
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "unable to provision tunnel"})
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func (a *ManagementAPI) authorized(r *http.Request) bool {
	if a.token == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return subtle.ConstantTimeCompare([]byte(a.token), []byte(provided)) == 1
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
