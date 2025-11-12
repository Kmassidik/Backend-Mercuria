package transaction

import (
	"net/http"

	"github.com/kmassidik/mercuria/internal/common/middleware"
)

func (h *Handler) RegisterRoutes(mux *http.ServeMux, jwtSecret string) {
	// Apply JWT auth to all transaction routes
	protected := middleware.JWTAuth(jwtSecret)

	mux.Handle("POST /api/v1/transactions", protected(http.HandlerFunc(h.CreateTransaction)))
	mux.Handle("POST /api/v1/transactions/batch", protected(http.HandlerFunc(h.CreateBatchTransaction)))
	mux.Handle("POST /api/v1/transactions/scheduled", protected(http.HandlerFunc(h.CreateScheduledTransaction)))
	mux.Handle("GET /api/v1/transactions/{id}", protected(http.HandlerFunc(h.GetTransaction)))
	mux.Handle("GET /api/v1/transactions", protected(http.HandlerFunc(h.ListTransactions)))
}