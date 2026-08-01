package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"golang.org/x/time/rate"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := chi.NewRouter()

	// 1. Core logger, security headers & max body size limit (1MB default)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(SecurityHeadersMiddleware)
	maxBodyBytes := GetEnvInt64("MAX_REQUEST_BODY_BYTES", 1<<20)
	r.Use(MaxRequestBodySizeMiddleware(maxBodyBytes))

	// 2. IP Blocking / Whitelisting middleware
	ipBlocker := NewIPBlocker(os.Getenv("BLOCKED_IPS"), os.Getenv("ALLOWED_IPS"))
	r.Use(ipBlocker.Handler)

	// 3. Proper CORS configuration
	r.Use(cors.Handler(BuildCORSConfig()))

	// 4. Global API Rate Limiter
	globalRPS := GetEnvFloat("RATE_LIMIT_RPS", 20.0)
	globalBurst := GetEnvInt("RATE_LIMIT_BURST", 50)
	globalLimiter := NewIPRateLimiter(rate.Limit(globalRPS), globalBurst)
	r.Use(globalLimiter.Handler)

	// Unrestricted / Health check endpoint
	r.Get("/", s.HelloWorldHandler)
	r.Get("/health", s.healthHandler)

	// Auth Subrouter with Stricter Rate Limiter
	authRPS := GetEnvFloat("AUTH_RATE_LIMIT_RPS", 2.0)
	authBurst := GetEnvInt("AUTH_RATE_LIMIT_BURST", 5)
	authLimiter := NewIPRateLimiter(rate.Limit(authRPS), authBurst)

	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(authLimiter.Handler)
		r.Get("/google/start", s.googleStartHandler)
		r.Get("/google/callback", s.googleCallbackHandler)
		r.Post("/google/exchange", s.googleExchangeHandler)
	})

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuthentication)
		r.Get("/api/v1/user/me", s.currentUserHandler)
		r.Get("/api/v1/plans", s.listPlansHandler)
		r.Get("/api/v1/tunnels/events", s.eventsHandler)
		r.Get("/api/v1/events", s.eventsHandler)
		r.Post("/api/v1/tunnels", s.createTunnelHandler)
		r.Delete("/api/v1/tunnels/{tunnelID}", s.deleteTunnelHandler)
	})
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(s.requireAuthentication)
		r.Use(requireAdministrator)
		r.Get("/users", s.adminListUsersHandler)
		r.Patch("/users/{userId}/plan", s.adminChangeUserPlanHandler)
		r.Post("/users/{userId}/reset-limits", s.adminResetUserLimitsHandler)
		r.Patch("/users/{userId}/admin", s.adminSetUserAdminHandler)
		r.Patch("/users/{userId}/ban", s.adminSetUserBannedHandler)
	})
	r.Post("/api/v1/tunnels/connect", s.connectTunnelHandler)
	r.Post("/internal/v1/tunnels/usage", s.syncTunnelUsageHandler)
	r.Post("/internal/v1/tunnels/status", s.syncTunnelConnectionStatusHandler)

	return r
}

func (s *Server) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	resp := make(map[string]string)
	resp["message"] = "Hello World"

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("error handling JSON marshal. Err: %v", err)
	}

	_, _ = w.Write(jsonResp)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	jsonResp, _ := json.Marshal(s.db.Health())
	_, _ = w.Write(jsonResp)
}
