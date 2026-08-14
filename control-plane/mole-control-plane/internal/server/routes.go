package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"golang.org/x/time/rate"
	serveradmin "mole-control-plane/internal/server/admin"
	serverauth "mole-control-plane/internal/server/auth"
	serverbilling "mole-control-plane/internal/server/billing"
	"mole-control-plane/internal/server/events"
	servermiddleware "mole-control-plane/internal/server/middleware"
	servertunnels "mole-control-plane/internal/server/tunnels"
	serverusers "mole-control-plane/internal/server/users"
	"net/http"
	"os"
)

func (s *Server) RegisterRoutes() http.Handler {
	if s.auth == nil {
		s.auth = serverauth.NewHandler(nil)
	}
	if s.billing == nil {
		s.billing = serverbilling.NewHandler(nil)
	}
	if s.admin == nil {
		s.admin = serveradmin.NewHandler(nil, nil)
	}
	if s.events == nil {
		s.events = events.NewHandler(nil, nil, events.NewBroker())
	}
	if s.tunnels == nil {
		s.tunnels = servertunnels.NewHandler(nil, nil, s.events)
	}
	if s.usersHandler == nil {
		s.usersHandler = serverusers.NewHandler(nil, nil, s.events)
	}
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer, servermiddleware.SecurityHeadersMiddleware)
	r.Use(servermiddleware.MaxRequestBodySizeMiddleware(servermiddleware.GetEnvInt64("MAX_REQUEST_BODY_BYTES", 1<<20)))
	r.Use(servermiddleware.NewIPBlocker(os.Getenv("BLOCKED_IPS"), os.Getenv("ALLOWED_IPS")).Handler)
	r.Use(cors.Handler(servermiddleware.BuildCORSConfig()))
	r.Use(servermiddleware.NewIPRateLimiter(rate.Limit(servermiddleware.GetEnvFloat("RATE_LIMIT_RPS", 20)), servermiddleware.GetEnvInt("RATE_LIMIT_BURST", 50)).Handler)
	r.Get("/", s.HelloWorldHandler)
	r.Get("/health", s.healthHandler)
	r.Post("/api/v1/billing/webhook", s.billing.StripeWebhook)
	authLimiter := servermiddleware.NewIPRateLimiter(rate.Limit(servermiddleware.GetEnvFloat("AUTH_RATE_LIMIT_RPS", 2)), servermiddleware.GetEnvInt("AUTH_RATE_LIMIT_BURST", 5))
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(authLimiter.Handler)
		r.Get("/google/start", s.auth.GoogleStart)
		r.Get("/google/callback", s.auth.GoogleCallback)
		r.Post("/google/exchange", s.auth.GoogleExchange)
	})
	r.Group(func(r chi.Router) {
		r.Use(serverauth.RequireAuthentication(s.users))
		r.Get("/api/v1/user/me", s.usersHandler.CurrentUser)
		r.Get("/api/v1/plans", s.usersHandler.ListPlans)
		r.Get("/api/v1/tunnels/events", s.events.Events)
		r.Get("/api/v1/events", s.events.Events)
		r.Post("/api/v1/tunnels", s.tunnels.Create)
		r.Delete("/api/v1/tunnels/{tunnelID}", s.tunnels.Delete)
		r.Post("/api/v1/billing/card-validation/setup", s.billing.CreateCardValidation)
		r.Post("/api/v1/billing/card-validation/confirm", s.billing.ConfirmCardValidation)
	})
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(serverauth.RequireAuthentication(s.users), serverauth.RequireAdministrator)
		r.Get("/users", s.admin.ListUsers)
		r.Patch("/users/{userId}/plan", s.admin.ChangeUserPlan)
		r.Post("/users/{userId}/reset-limits", s.admin.ResetUserLimits)
		r.Patch("/users/{userId}/admin", s.admin.SetUserAdmin)
		r.Patch("/users/{userId}/ban", s.admin.SetUserBanned)
	})
	r.Post("/api/v1/tunnels/connect", s.tunnels.Connect)
	r.Post("/internal/v1/tunnels/usage", s.tunnels.SyncUsage)
	r.Post("/internal/v1/tunnels/status", s.tunnels.SyncConnectionStatus)
	return r
}
