package tunnels

import (
	"context"
	"mole-control-plane/internal/server/events"
	"mole-control-plane/internal/tunnel"
)

type Handler struct {
	Service  *tunnel.Service
	SetupErr error
	Events   *events.Handler
}

func NewHandler(service *tunnel.Service, setupErr error, eventHandler *events.Handler) *Handler {
	return &Handler{Service: service, SetupErr: setupErr, Events: eventHandler}
}
func (h *Handler) NotifyUserUpdate(ctx context.Context, userID string) {
	h.Events.NotifyUserUpdate(ctx, userID)
}
func (h *Handler) NotifyUserUpdateForTunnel(ctx context.Context, tunnelID string) {
	h.Events.NotifyUserUpdateForTunnel(ctx, tunnelID)
}
func (h *Handler) NotifyUserUpdateForUsage(ctx context.Context, updates []tunnel.UsageUpdate) {
	h.Events.NotifyUserUpdateForUsage(ctx, updates)
}
