package users

import (
	"mole-control-plane/internal/server/events"
	"mole-control-plane/internal/tunnel"
	"mole-control-plane/internal/user"
)

type Handler struct {
	Users   *user.Service
	Tunnels *tunnel.Service
	Events  *events.Handler
}

func NewHandler(users *user.Service, tunnels *tunnel.Service, eventHandler *events.Handler) *Handler {
	return &Handler{Users: users, Tunnels: tunnels, Events: eventHandler}
}
