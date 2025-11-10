package auth

import (
	"net/http"

	"github.com/kmassidik/mercuria/internal/common/middleware"
)

func (h *Handler) RegisterRoutes(mux *http.ServeMux, jwtSecret string) {
	// Public routes
	mux.HandleFunc("POST /api/v1/register", h.Register)
	mux.HandleFunc("POST /api/v1/login", h.Login)
	mux.HandleFunc("POST /api/v1/refresh", h.Refresh)

	// Protected routes
	protected := middleware.JWTAuth(jwtSecret)
	mux.Handle("GET /api/v1/me", protected(http.HandlerFunc(h.Me)))
}