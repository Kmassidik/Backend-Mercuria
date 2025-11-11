package wallet

import (
	"net/http"

	"github.com/kmassidik/mercuria/internal/common/middleware"
)

func (h *Handler) RegisterRoutes(mux *http.ServeMux, jwtSecret string) {
	// Apply JWT auth to all wallet routes
	protected := middleware.JWTAuth(jwtSecret)

	// Wallet routes
	mux.Handle("POST /api/v1/wallets", protected(http.HandlerFunc(h.CreateWallet)))
	mux.Handle("GET /api/v1/wallets/{id}", protected(http.HandlerFunc(h.GetWallet)))
	mux.Handle("POST /api/v1/wallets/{id}/deposit", protected(http.HandlerFunc(h.Deposit)))
	mux.Handle("POST /api/v1/wallets/{id}/withdraw", protected(http.HandlerFunc(h.Withdraw)))
	mux.Handle("GET /api/v1/wallets/{id}/events", protected(http.HandlerFunc(h.GetWalletEvents)))
}