// Package httpx contains shared HTTP transport helpers.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const MaxJSONRequestBytes = 8 << 10

func DecodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxJSONRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one JSON object"})
		return err
	}
	return nil
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
