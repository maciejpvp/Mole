package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/", s.HelloWorldHandler)

	r.Get("/health", s.healthHandler)
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/register", s.registerHandler)
		r.Post("/login", s.loginHandler)
	})
	r.Get("/api/v1/user/me", s.currentUserHandler)
	r.Get("/api/v1/tunnels/events", s.eventsHandler)
	r.Get("/api/v1/events", s.eventsHandler)
	r.Post("/api/v1/tunnels", s.createTunnelHandler)
	r.Delete("/api/v1/tunnels/{tunnelID}", s.deleteTunnelHandler)
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
