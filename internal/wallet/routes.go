package wallet

import (
	"net/http"

	"github.com/kmassidik/mercuria/internal/common/middleware"
)

// RegisterMtlsRoutes handles routes intended for internal, mTLS-only communication.
func (h *Handler) RegisterMtlsRoutes(mux *http.ServeMux) {
	// Transfer should likely only be called by the Transaction Service, so it belongs here.
	// JWTAuth is REMOVED. Access is controlled solely by mTLS handshake validation.
	mux.HandleFunc("POST /api/v1/wallets/transfer", h.Transfer)
    // You can also move any other internal service-to-service calls here.
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, jwtSecret string) {
	// Apply JWT auth to all wallet routes for external/public access
	protected := middleware.JWTAuth(jwtSecret)

	// Wallet routes (Public/JWT-protected)
	mux.Handle("POST /api/v1/wallets", protected(http.HandlerFunc(h.CreateWallet)))
	mux.Handle("GET /api/v1/wallets/{id}", protected(http.HandlerFunc(h.GetWallet)))
	mux.Handle("POST /api/v1/wallets/{id}/deposit", protected(http.HandlerFunc(h.Deposit)))
	mux.Handle("POST /api/v1/wallets/{id}/withdraw", protected(http.HandlerFunc(h.Withdraw)))
	mux.Handle("GET /api/v1/wallets/{id}/events", protected(http.HandlerFunc(h.GetWalletEvents)))
	mux.Handle("GET /api/v1/wallets/my-wallets", protected(http.HandlerFunc(h.GetMyWallets))) // This is the route that was failing!

	// Internal route (if it's JWT protected)
	mux.Handle("GET /api/v1/internal/wallets/{id}", protected(http.HandlerFunc(h.GetWalletInternal)))
	// REMOVED: mux.Handle("POST /api/v1/wallets/transfer", ...), as it moved to RegisterMtlsRoutes
}