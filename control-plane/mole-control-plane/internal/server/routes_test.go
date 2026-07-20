package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler(t *testing.T) {
	s := &Server{}
	recorder := httptest.NewRecorder()
	s.HelloWorldHandler(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	resp := recorder.Result()
	defer resp.Body.Close()
	// Assertions
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status OK; got %v", resp.Status)
	}
	expected := "{\"message\":\"Hello World\"}"
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading response body. Err: %v", err)
	}
	if expected != string(body) {
		t.Errorf("expected response body to be %v; got %v", expected, string(body))
	}
}

func TestCurrentUserRouteRequiresAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/user/me", nil))
	resp := recorder.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status unauthorized; got %v", resp.Status)
	}
}
