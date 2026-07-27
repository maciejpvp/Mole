package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"mole-control-plane/internal/admin"
)

func (s *Server) listAdminUsersHandler(w http.ResponseWriter, r *http.Request) {
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
