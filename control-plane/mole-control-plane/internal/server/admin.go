package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"mole-control-plane/internal/admin"
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

func (s *Server) adminListUsersHandler(w http.ResponseWriter, r *http.Request) {
	limit, err := parseAdminLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 100"})
		return
	}

	page, err := s.admin.ListUsers(r.Context(), admin.ListUsersInput{
		Search:    r.URL.Query().Get("search"),
		Limit:     limit,
		Cursor:    r.URL.Query().Get("cursor"),
		Sort:      admin.SortField(strings.ToLower(r.URL.Query().Get("sort"))),
		Direction: admin.SortDirection(strings.ToLower(r.URL.Query().Get("direction"))),
	})
	if err != nil {
		if errors.Is(err, admin.ErrInvalidInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pagination, search, or sort parameters"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to list users"})
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) adminChangeUserPlanHandler(w http.ResponseWriter, r *http.Request) {
	var request changeUserPlanRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	if request.PlanID < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id must be a positive integer"})
		return
	}

	account, err := s.admin.ChangeUserPlan(r.Context(), chi.URLParam(r, "userId"), request.PlanID)
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userId and plan_id are required"})
		case errors.Is(err, admin.ErrUserNotFound), errors.Is(err, admin.ErrPlanNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to change user plan"})
		}
		return
	}

	writeJSON(w, http.StatusOK, account)
}

func (s *Server) adminResetUserLimitsHandler(w http.ResponseWriter, r *http.Request) {
	account, err := s.admin.ResetUserLimits(r.Context(), chi.URLParam(r, "userId"))
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
		case errors.Is(err, admin.ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to reset user limits"})
		}
		return
	}

	writeJSON(w, http.StatusOK, account)
}

func (s *Server) adminSetUserAdminHandler(w http.ResponseWriter, r *http.Request) {
	var request setUserAdminRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	if request.IsAdmin == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "is_admin is required"})
		return
	}

	account, err := s.admin.SetUserAdmin(r.Context(), chi.URLParam(r, "userId"), *request.IsAdmin)
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
		case errors.Is(err, admin.ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to update user administrator permission"})
		}
		return
	}

	writeJSON(w, http.StatusOK, account)
}

func (s *Server) adminSetUserBannedHandler(w http.ResponseWriter, r *http.Request) {
	var request setUserBannedRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	if request.IsBanned == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "is_banned is required"})
		return
	}

	account, err := s.admin.SetUserBanned(r.Context(), chi.URLParam(r, "userId"), *request.IsBanned)
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
		case errors.Is(err, admin.ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, admin.ErrUnavailable), errors.Is(err, admin.ErrTunnelCleanup):
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "unable to remove user tunnels"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to update user ban status"})
		}
		return
	}

	if *request.IsBanned && s.broker != nil {
		s.broker.Broadcast(account.ID, Event{
			Name: "user_banned",
			Data: map[string]bool{"is_banned": true},
		})
	}
	writeJSON(w, http.StatusOK, account)
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
