package transaction

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/middleware"
)

type ServiceInterface interface {
	CreateP2PTransfer(ctx context.Context, req *CreateTransactionRequest) (*Transaction, error)
	CreateBatchTransfer(ctx context.Context, req *CreateBatchTransactionRequest) (*BatchTransaction, []Transaction, error)
	CreateScheduledTransfer(ctx context.Context, req *CreateScheduledTransactionRequest) (*Transaction, error)
	GetTransaction(ctx context.Context, id string) (*Transaction, error)
	ListTransactionsByWallet(ctx context.Context, walletID string, limit, offset int) ([]Transaction, error)
}

type Handler struct {
	service ServiceInterface
	logger  *logger.Logger
}

func NewHandler(service ServiceInterface, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// CreateTransaction handles P2P transfer creation
// NOTE: Most common endpoint - simple wallet-to-wallet transfer
func (h *Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// TODO: Verify user owns from_wallet_id (wallet service integration)
	_ = userID

	// IMPORTANT: Get auth header and add to context for inter-service calls
	ctx := r.Context()
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		h.logger.Infof("Found Authorization header in request: %s...", authHeader[:20])
		ctx = SetAuthorizationInContext(ctx, authHeader)
	} else {
		h.logger.Warn("No Authorization header found in request!")
	}

	txn, err := h.service.CreateP2PTransfer(ctx, &req)
	if err != nil {
		h.logger.Errorf("Failed to create transfer: %v", err)
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusCreated, TransactionResponse{Transaction: txn})
}

// CreateBatchTransaction handles batch transfer creation
// NOTE: Multiple recipients in one request - atomic execution
func (h *Handler) CreateBatchTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateBatchTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// TODO: Verify user owns from_wallet_id
	_ = userID

	// Add Authorization header to context for inter-service calls
	ctx := r.Context()
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		ctx = SetAuthorizationInContext(ctx, authHeader)
	}

	batch, txns, err := h.service.CreateBatchTransfer(ctx, &req)
	if err != nil {
		h.logger.Errorf("Failed to create batch transfer: %v", err)
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusCreated, BatchTransactionResponse{
		BatchTransaction: batch,
		Transactions:     txns,
	})
}

// CreateScheduledTransaction handles scheduled transfer creation
// NOTE: Future-dated transfer - executed by background worker
func (h *Handler) CreateScheduledTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateScheduledTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// TODO: Verify user owns from_wallet_id
	_ = userID

	// Add Authorization header to context for inter-service calls
	ctx := r.Context()
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		ctx = SetAuthorizationInContext(ctx, authHeader)
	}

	txn, err := h.service.CreateScheduledTransfer(ctx, &req)
	if err != nil {
		h.logger.Errorf("Failed to create scheduled transfer: %v", err)
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusCreated, TransactionResponse{Transaction: txn})
}

// GetTransaction retrieves a transaction by ID
func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	txnID := r.PathValue("id")
	if txnID == "" {
		h.respondError(w, http.StatusBadRequest, "transaction ID is required")
		return
	}

	// Add Authorization header to context for inter-service calls
	ctx := r.Context()
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		ctx = SetAuthorizationInContext(ctx, authHeader)
	}

	txn, err := h.service.GetTransaction(ctx, txnID)
	if err != nil {
		h.logger.Errorf("Failed to get transaction: %v", err)
		h.respondError(w, http.StatusNotFound, "transaction not found")
		return
	}

	// TODO: Verify user owns one of the wallets in transaction
	_ = userID

	h.respondJSON(w, http.StatusOK, TransactionResponse{Transaction: txn})
}

// ListTransactions lists transactions for a wallet
func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	walletID := r.URL.Query().Get("wallet_id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet_id query parameter is required")
		return
	}

	// Parse pagination
	limit := 50
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// TODO: Verify user owns wallet
	_ = userID

	// Add Authorization header to context for inter-service calls
	ctx := r.Context()
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		ctx = SetAuthorizationInContext(ctx, authHeader)
	}

	txns, err := h.service.ListTransactionsByWallet(ctx, walletID, limit, offset)
	if err != nil {
		h.logger.Errorf("Failed to list transactions: %v", err)
		h.respondError(w, http.StatusInternalServerError, "failed to list transactions")
		return
	}

	h.respondJSON(w, http.StatusOK, TransactionListResponse{
		Transactions: txns,
		Total:        len(txns),
		Limit:        limit,
		Offset:       offset,
	})
}

// Helper methods
func (h *Handler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, ErrorResponse{Error: message})
}