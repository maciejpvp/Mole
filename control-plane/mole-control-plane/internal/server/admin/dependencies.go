package admin

import (
	"mole-control-plane/internal/admin"
	"mole-control-plane/internal/server/events"
)

type Handler struct {
	Service *admin.Service
	Broker  *events.Broker
}

func NewHandler(service *admin.Service, broker *events.Broker) *Handler {
	return &Handler{Service: service, Broker: broker}
}
