package server

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"mole-control-plane/internal/billing"
)

type confirmCardRequest struct {
	SetupIntentID string `json:"setup_intent_id"`
}

func (s *Server) createCardValidationHandler(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing service unavailable"})
		return
	}
	result, err := s.billing.CreateCardValidation(r.Context(), userFromContext(r.Context()))
	if err != nil {
		billingError(w, err, "unable to create card validation")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) confirmCardValidationHandler(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing service unavailable"})
		return
	}
	var request confirmCardRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	result, err := s.billing.ConfirmCardValidation(r.Context(), userFromContext(r.Context()), request.SetupIntentID)
	if err != nil {
		if errors.Is(err, billing.ErrInvalidStatus) {
			writeJSON(w, http.StatusConflict, result)
			return
		}
		billingError(w, err, "unable to confirm card validation")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) stripeWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing service unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook body"})
		return
	}
	if err := s.billing.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature")); err != nil {
		if errors.Is(err, billing.ErrNotConfigured) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing service unavailable"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Stripe webhook"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"received": "true"})
}

func billingError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, billing.ErrNotConfigured):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing is not configured"})
	case errors.Is(err, billing.ErrOwnership):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "billing object does not belong to user"})
	case errors.Is(err, billing.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "billing record not found"})
	default:
		if strings.TrimSpace(fallback) == "" {
			fallback = "billing request failed"
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fallback})
	}
}
