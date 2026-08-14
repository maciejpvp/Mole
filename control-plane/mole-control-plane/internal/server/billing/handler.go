package billing

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"mole-control-plane/internal/billing"
	"mole-control-plane/internal/server/auth"
	"mole-control-plane/internal/server/httpx"
)

type confirmCardRequest struct {
	SetupIntentID string `json:"setup_intent_id"`
}

func (h *Handler) CreateCardValidation(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing service unavailable"})
		return
	}
	result, err := h.Service.CreateCardValidation(r.Context(), auth.UserFromContext(r.Context()))
	if err != nil {
		billingError(w, err, "unable to create card validation")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) ConfirmCardValidation(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing service unavailable"})
		return
	}
	var request confirmCardRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		return
	}
	result, err := h.Service.ConfirmCardValidation(r.Context(), auth.UserFromContext(r.Context()), request.SetupIntentID)
	if err != nil {
		if errors.Is(err, billing.ErrInvalidStatus) {
			httpx.WriteJSON(w, http.StatusConflict, result)
			return
		}
		billingError(w, err, "unable to confirm card validation")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing service unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook body"})
		return
	}
	if err := h.Service.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature")); err != nil {
		if errors.Is(err, billing.ErrNotConfigured) {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing service unavailable"})
			return
		}
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Stripe webhook"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"received": "true"})
}

func billingError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, billing.ErrNotConfigured):
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing is not configured"})
	case errors.Is(err, billing.ErrOwnership):
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "billing object does not belong to user"})
	case errors.Is(err, billing.ErrNotFound):
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "billing record not found"})
	default:
		if strings.TrimSpace(fallback) == "" {
			fallback = "billing request failed"
		}
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": fallback})
	}
}
