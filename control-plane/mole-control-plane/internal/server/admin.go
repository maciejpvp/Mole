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
