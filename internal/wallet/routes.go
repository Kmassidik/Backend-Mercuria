package wallet

import (
	"net/http"

	"github.com/kmassidik/mercuria/internal/common/middleware"
)

// RegisterRoutes - PUBLIC API (HTTPS + JWT for external clients)
func (h *Handler) RegisterRoutes(mux *http.ServeMux, jwtSecret string) {
	protected := middleware.JWTAuth(jwtSecret)

	mux.Handle("POST /api/v1/wallets", protected(http.HandlerFunc(h.CreateWallet)))
	mux.Handle("GET /api/v1/wallets/{id}", protected(http.HandlerFunc(h.GetWallet)))
	mux.Handle("POST /api/v1/wallets/{id}/deposit", protected(http.HandlerFunc(h.Deposit)))
	mux.Handle("POST /api/v1/wallets/{id}/withdraw", protected(http.HandlerFunc(h.Withdraw)))
	mux.Handle("GET /api/v1/wallets/{id}/events", protected(http.HandlerFunc(h.GetWalletEvents)))
	mux.Handle("GET /api/v1/wallets/my-wallets", protected(http.HandlerFunc(h.GetMyWallets)))
}

// RegisterInternalRoutes - INTERNAL API (mTLS only, NO JWT needed)
func (h *Handler) RegisterInternalRoutes(mux *http.ServeMux) {
	// These routes are ONLY accessible via mTLS on internal port
	// Certificate verification = authentication
	mux.HandleFunc("GET /api/v1/internal/wallets/{id}", h.GetWalletInternal)
	mux.HandleFunc("POST /api/v1/internal/wallets/transfer", h.Transfer)
}