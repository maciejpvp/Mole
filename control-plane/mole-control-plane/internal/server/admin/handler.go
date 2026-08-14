package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"mole-control-plane/internal/admin"
	"mole-control-plane/internal/server/events"
	"mole-control-plane/internal/server/httpx"
)

type changeUserPlanRequest struct {
	PlanID int64 `json:"plan_id"`
}

type setUserAdminRequest struct {
	IsAdmin *bool `json:"is_admin"`
}

type setUserBannedRequest struct {
	IsBanned *bool `json:"is_banned"`
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, err := parseAdminLimit(r.URL.Query().Get("limit"))
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 100"})
		return
	}

	page, err := h.Service.ListUsers(r.Context(), admin.ListUsersInput{
		Search:    r.URL.Query().Get("search"),
		Limit:     limit,
		Cursor:    r.URL.Query().Get("cursor"),
		Sort:      admin.SortField(strings.ToLower(r.URL.Query().Get("sort"))),
		Direction: admin.SortDirection(strings.ToLower(r.URL.Query().Get("direction"))),
	})
	if err != nil {
		if errors.Is(err, admin.ErrInvalidInput) {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pagination, search, or sort parameters"})
			return
		}
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to list users"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) ChangeUserPlan(w http.ResponseWriter, r *http.Request) {
	var request changeUserPlanRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		return
	}
	if request.PlanID < 1 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id must be a positive integer"})
		return
	}

	account, err := h.Service.ChangeUserPlan(r.Context(), chi.URLParam(r, "userId"), request.PlanID)
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "userId and plan_id are required"})
		case errors.Is(err, admin.ErrUserNotFound), errors.Is(err, admin.ErrPlanNotFound):
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to change user plan"})
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, account)
}

func (h *Handler) ResetUserLimits(w http.ResponseWriter, r *http.Request) {
	account, err := h.Service.ResetUserLimits(r.Context(), chi.URLParam(r, "userId"))
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
		case errors.Is(err, admin.ErrUserNotFound):
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to reset user limits"})
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, account)
}

func (h *Handler) SetUserAdmin(w http.ResponseWriter, r *http.Request) {
	var request setUserAdminRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		return
	}
	if request.IsAdmin == nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "is_admin is required"})
		return
	}

	account, err := h.Service.SetUserAdmin(r.Context(), chi.URLParam(r, "userId"), *request.IsAdmin)
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
		case errors.Is(err, admin.ErrUserNotFound):
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to update user administrator permission"})
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, account)
}

func (h *Handler) SetUserBanned(w http.ResponseWriter, r *http.Request) {
	var request setUserBannedRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		return
	}
	if request.IsBanned == nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "is_banned is required"})
		return
	}

	account, err := h.Service.SetUserBanned(r.Context(), chi.URLParam(r, "userId"), *request.IsBanned)
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
		case errors.Is(err, admin.ErrUserNotFound):
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, admin.ErrUnavailable), errors.Is(err, admin.ErrTunnelCleanup):
			httpx.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "unable to remove user tunnels"})
		default:
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to update user ban status"})
		}
		return
	}

	if *request.IsBanned && h.Broker != nil {
		h.Broker.Broadcast(account.ID, events.Event{
			Name: "user_banned",
			Data: map[string]bool{"is_banned": true},
		})
	}
	httpx.WriteJSON(w, http.StatusOK, account)
}

func parseAdminLimit(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return limit, nil
}
