package events

import (
	"context"
	"mole-control-plane/internal/tunnel"
	"mole-control-plane/internal/user"
)

type Handler struct {
	Users   *user.Service
	Tunnels *tunnel.Service
	Broker  *Broker
}

func NewHandler(users *user.Service, tunnels *tunnel.Service, broker *Broker) *Handler {
	return &Handler{Users: users, Tunnels: tunnels, Broker: broker}
}
func (h *Handler) NotifyUserUpdate(ctx context.Context, userID string) {
	h.notifyUserUpdate(ctx, userID)
}
func (h *Handler) NotifyUserUpdateForTunnel(ctx context.Context, tunnelID string) {
	h.notifyUserUpdateForTunnel(ctx, tunnelID)
}
func (h *Handler) NotifyUserUpdateForUsage(ctx context.Context, updates []tunnel.UsageUpdate) {
	h.notifyUserUpdateForUsage(ctx, updates)
}
