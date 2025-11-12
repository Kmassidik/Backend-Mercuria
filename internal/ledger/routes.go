package ledger

import (
	"net/http"

	"github.com/kmassidik/mercuria/internal/common/middleware"
)

func (h *Handler) RegisterRoutes(mux *http.ServeMux, jwtSecret string) {
	protected := middleware.JWTAuth(jwtSecret)

	// Ledger routes (read-only)
	mux.Handle("GET /api/v1/ledger/{id}", protected(http.HandlerFunc(h.GetLedgerEntry)))
	mux.Handle("GET /api/v1/ledger/transaction/{id}", protected(http.HandlerFunc(h.GetTransactionLedger)))
	mux.Handle("GET /api/v1/ledger/wallet", protected(http.HandlerFunc(h.GetWalletLedger)))
	mux.Handle("GET /api/v1/ledger/stats", protected(http.HandlerFunc(h.GetWalletStats)))
	mux.Handle("GET /api/v1/ledger", protected(http.HandlerFunc(h.GetAllEntries)))
}
