package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestGetClientIP(t *testing.T) {
	req1, _ := http.NewRequest("GET", "/", nil)
	req1.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
	if ip := GetClientIP(req1); ip != "203.0.113.195" {
		t.Errorf("expected 203.0.113.195, got %s", ip)
	}

	req2, _ := http.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Real-IP", "198.51.100.1")
	if ip := GetClientIP(req2); ip != "198.51.100.1" {
		t.Errorf("expected 198.51.100.1, got %s", ip)
	}

	req3, _ := http.NewRequest("GET", "/", nil)
	req3.RemoteAddr = "192.0.2.1:12345"
	if ip := GetClientIP(req3); ip != "192.0.2.1" {
		t.Errorf("expected 192.0.2.1, got %s", ip)
	}
}

func TestIPBlocker(t *testing.T) {
	blocker := NewIPBlocker("10.0.0.1,192.168.1.0/24", "")

	if !blocker.IsBlocked("10.0.0.1") {
		t.Errorf("10.0.0.1 should be blocked")
	}
	if !blocker.IsBlocked("192.168.1.50") {
		t.Errorf("192.168.1.50 should be blocked by CIDR 192.168.1.0/24")
	}
	if blocker.IsBlocked("10.0.0.2") {
		t.Errorf("10.0.0.2 should not be blocked")
	}

	whitelistBlocker := NewIPBlocker("", "172.16.0.0/12")
	if whitelistBlocker.IsBlocked("172.16.5.10") {
		t.Errorf("172.16.5.10 is whitelisted, should not be blocked")
	}
	if !whitelistBlocker.IsBlocked("8.8.8.8") {
		t.Errorf("8.8.8.8 is not whitelisted, should be blocked")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	limiter := NewIPRateLimiter(rate.Limit(2), 2) // 2 req/sec, burst 2
	defer close(limiter.stopChan)

	handler := limiter.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1000"

	// First two requests should pass
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec2.Code)
	}

	// Third immediate request should be rate limited (429)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 Too Many Requests, got %d", rec3.Code)
	}
	if rec3.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header to be set")
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	headers := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Referrer-Policy",
	}

	for _, h := range headers {
		if rec.Header().Get(h) == "" {
			t.Errorf("expected security header %s to be present", h)
		}
	}
}

func TestCORSIntegration(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.mole.com")

	s := &Server{}
	r := s.RegisterRoutes()

	req, _ := http.NewRequest("OPTIONS", "/health", nil)
	req.Header.Set("Origin", "https://app.mole.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.mole.com" {
		t.Errorf("expected Access-Control-Allow-Origin to be https://app.mole.com, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	// Unauthorized origin test
	req2, _ := http.NewRequest("OPTIONS", "/health", nil)
	req2.Header.Set("Origin", "https://malicious.com")
	req2.Header.Set("Access-Control-Request-Method", "GET")

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if rec2.Header().Get("Access-Control-Allow-Origin") == "https://malicious.com" {
		t.Errorf("unauthorized origin should not receive Access-Control-Allow-Origin header")
	}
}

func TestAuthRateLimitingIntegration(t *testing.T) {
	t.Setenv("AUTH_RATE_LIMIT_RPS", "1")
	t.Setenv("AUTH_RATE_LIMIT_BURST", "1")

	s := &Server{}
	router := s.RegisterRoutes()

	makeReq := func() int {
		req, _ := http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"identifier":"a","password":"b"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.10.10.10:5555"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	code1 := makeReq()
	if code1 == http.StatusTooManyRequests {
		t.Errorf("first auth request should not be rate limited")
	}

	code2 := makeReq()
	if code2 != http.StatusTooManyRequests {
		t.Errorf("second immediate auth request should be rate limited with status 429, got %d", code2)
	}

	// Sleep for 1 second to replenish token
	time.Sleep(1100 * time.Millisecond)

	code3 := makeReq()
	if code3 == http.StatusTooManyRequests {
		t.Errorf("request after replenish window should pass, got %d", code3)
	}
}

func TestMaxRequestBodySizeMiddleware(t *testing.T) {
	middleware := MaxRequestBodySizeMiddleware(10) // 10 bytes max
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 100)
		_, err := r.Body.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	largeBody := strings.NewReader("12345678901234567890")
	req, _ := http.NewRequest("POST", "/", largeBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 StatusRequestEntityTooLarge for oversized payload, got %d", rec.Code)
	}
}
