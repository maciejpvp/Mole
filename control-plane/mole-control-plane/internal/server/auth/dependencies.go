package auth

import "mole-control-plane/internal/user"

type Handler struct{ Users *user.Service }

func NewHandler(users *user.Service) *Handler { return &Handler{Users: users} }
