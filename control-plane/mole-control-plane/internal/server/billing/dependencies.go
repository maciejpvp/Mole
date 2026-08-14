package billing

import "mole-control-plane/internal/billing"

type Handler struct{ Service *billing.Service }

func NewHandler(service *billing.Service) *Handler { return &Handler{Service: service} }
