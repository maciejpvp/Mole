package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"mole-control-plane/internal/admin"
	"mole-control-plane/internal/billing"
	"mole-control-plane/internal/database"
	serveradmin "mole-control-plane/internal/server/admin"
	serverauth "mole-control-plane/internal/server/auth"
	serverbilling "mole-control-plane/internal/server/billing"
	"mole-control-plane/internal/server/events"
	servertunnels "mole-control-plane/internal/server/tunnels"
	serverusers "mole-control-plane/internal/server/users"
	"mole-control-plane/internal/tunnel"
	"mole-control-plane/internal/user"
)

// Server composes transport handlers and infrastructure.
type Server struct {
	db           database.Service
	users        *user.Service
	auth         *serverauth.Handler
	billing      *serverbilling.Handler
	admin        *serveradmin.Handler
	tunnels      *servertunnels.Handler
	usersHandler *serverusers.Handler
	events       *events.Handler
}

func NewServer() *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	db := database.New()
	users := user.NewService(db.DB())
	provisioner, setupErr := tunnel.NewHTTPProvisionerFromEnv()
	tunnelService := tunnel.NewService(db.DB(), provisioner)
	broker := events.NewBroker()
	eventHandler := events.NewHandler(users, tunnelService, broker)
	api := &Server{
		db: db, users: users,
		auth:         serverauth.NewHandler(users),
		billing:      serverbilling.NewHandler(billing.NewService(db.DB(), os.Getenv("STRIPE_SECRET_KEY"), os.Getenv("STRIPE_WEBHOOK_SECRET"))),
		admin:        serveradmin.NewHandler(admin.NewService(db.DB(), provisioner), broker),
		tunnels:      servertunnels.NewHandler(tunnelService, setupErr, eventHandler),
		usersHandler: serverusers.NewHandler(users, tunnelService, eventHandler),
		events:       eventHandler,
	}
	return &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: api.RegisterRoutes(), IdleTimeout: time.Minute, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
}

func (s *Server) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	response, _ := json.Marshal(map[string]string{"message": "Hello World"})
	_, _ = w.Write(response)
}
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(s.db.Health())
}
