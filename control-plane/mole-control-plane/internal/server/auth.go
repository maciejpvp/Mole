package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"mole-control-plane/internal/user"
)

const maxAuthRequestBytes = 8 << 10

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

func (s *Server) registerHandler(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}

	authentication, err := s.users.Register(r.Context(), user.RegisterInput{
		Username: request.Username,
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, user.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username, email, and password are invalid"})
		case errors.Is(err, user.ErrAccountUnavailable):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username or email is already in use"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to register"})
		}
		return
	}

	writeJSON(w, http.StatusCreated, authentication)
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}

	authentication, err := s.users.Login(r.Context(), user.LoginInput{
		Identifier: request.Identifier,
		Password:   request.Password,
	})
	if err != nil {
		if errors.Is(err, user.ErrInvalidCredentials) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to log in"})
		return
	}

	writeJSON(w, http.StatusOK, authentication)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one JSON object"})
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
