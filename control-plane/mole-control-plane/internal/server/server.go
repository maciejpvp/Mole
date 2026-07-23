package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"mole-control-plane/internal/database"
	"mole-control-plane/internal/tunnel"
	"mole-control-plane/internal/user"
)

type Server struct {
	port int

	db database.Service

	users   *user.Service
	tunnels *tunnel.Service
	broker  *Broker

	tunnelSetupErr error
}

func NewServer() *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	NewServer := &Server{
		port: port,

		db:     database.New(),
		broker: NewBroker(),
	}
	NewServer.users = user.NewService(NewServer.db.DB())
	provisioner, err := tunnel.NewHTTPProvisionerFromEnv()
	NewServer.tunnels = tunnel.NewService(NewServer.db.DB(), provisioner)
	NewServer.tunnelSetupErr = err

	// Declare Server config
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", NewServer.port),
		Handler:      NewServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return server
}
