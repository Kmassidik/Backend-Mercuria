package wallet

import (
	"context" // <-- You'll need this import
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/middleware"
)

// +FIX 1: Define the Service Interface
// This interface defines the contract our handler depends on.
type ServiceInterface interface {
	CreateWallet(ctx context.Context, req *CreateWalletRequest) (*Wallet, error)
	GetWallet(ctx context.Context, walletID string) (*Wallet, error)
	Deposit(ctx context.Context, walletID string, req *DepositRequest) (*Wallet, error)
	Withdraw(ctx context.Context, walletID string, req *WithdrawRequest) (*Wallet, error)
	GetWalletEvents(ctx context.Context, walletID string, limit, offset int) ([]WalletEvent, error)
	GetWalletsByUserID(ctx context.Context, userID string) ([]Wallet, error) // <- ADD THIS
	Transfer(ctx context.Context, req *TransferRequest) error // <- ADD THIS
}

type Handler struct {
	// +FIX 2: Depend on the interface, not the concrete *Service
	service ServiceInterface
	logger  *logger.Logger
}

// +FIX 3: Accept the interface as a parameter
func NewHandler(service ServiceInterface, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// CreateWallet handles wallet creation
func (h *Handler) CreateWallet(w http.ResponseWriter, r *http.Request) {
    // ... (rest of your handler code is correct) ...
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.UserID = userID

	wallet, err := h.service.CreateWallet(r.Context(), &req)
	if err != nil {
		h.logger.Errorf("Failed to create wallet: %v", err)
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusCreated, WalletResponse{Wallet: wallet})
}

// GetWallet handles wallet retrieval
func (h *Handler) GetWallet(w http.ResponseWriter, r *http.Request) {
    // ... (rest of your handler code is correct) ...
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	wallet, err := h.service.GetWallet(r.Context(), walletID)
	if err != nil {
		h.logger.Errorf("Failed to get wallet: %v", err)
		h.respondError(w, http.StatusNotFound, "wallet not found")
		return
	}

	if wallet.UserID != userID {
		h.respondError(w, http.StatusForbidden, "access denied")
		return
	}

	h.respondJSON(w, http.StatusOK, WalletResponse{Wallet: wallet})
}

// Deposit handles deposit requests
func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
    // ... (rest of your handler code is correct) ...
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req DepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	wallet, err := h.service.GetWallet(r.Context(), walletID)
	if err != nil {
		h.respondError(w, http.StatusNotFound, "wallet not found")
		return
	}

	if wallet.UserID != userID {
		h.respondError(w, http.StatusForbidden, "access denied")
		return
	}

	updatedWallet, err := h.service.Deposit(r.Context(), walletID, &req)
	if err != nil {
		h.logger.Errorf("Deposit failed: %v", err)
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, WalletResponse{Wallet: updatedWallet})
}

// Withdraw handles withdrawal requests
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
    // ... (rest of your handler code is correct) ...
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req WithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	wallet, err := h.service.GetWallet(r.Context(), walletID)
	if err != nil {
		h.respondError(w, http.StatusNotFound, "wallet not found")
		return
	}

	if wallet.UserID != userID {
		h.respondError(w, http.StatusForbidden, "access denied")
		return
	}

	updatedWallet, err := h.service.Withdraw(r.Context(), walletID, &req)
	if err != nil {
		h.logger.Errorf("Withdrawal failed: %v", err)
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, WalletResponse{Wallet: updatedWallet})
}

// GetWalletEvents handles wallet event history retrieval
func (h *Handler) GetWalletEvents(w http.ResponseWriter, r *http.Request) {
    // ... (rest of your handler code is correct) ...
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	wallet, err := h.service.GetWallet(r.Context(), walletID)
	if err != nil {
		h.respondError(w, http.StatusNotFound, "wallet not found")
		return
	}

	if wallet.UserID != userID {
		h.respondError(w, http.StatusForbidden, "access denied")
		return
	}

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

	events, err := h.service.GetWalletEvents(r.Context(), walletID, limit, offset)
	if err != nil {
		h.logger.Errorf("Failed to get events: %v", err)
		h.respondError(w, http.StatusInternalServerError, "failed to get events")
		return
	}

	h.respondJSON(w, http.StatusOK, WalletEventsResponse{
		Events: events,
		Total:  len(events),
	})
}

// GetMyWallets handles retrieving all wallets for authenticated user
func (h *Handler) GetMyWallets(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	wallets, err := h.service.GetWalletsByUserID(r.Context(), userID)
	if err != nil {
		h.logger.Errorf("Failed to get user wallets: %v", err)
		h.respondError(w, http.StatusInternalServerError, "failed to get wallets")
		return
	}

	h.respondJSON(w, http.StatusOK, MyWalletsResponse{
		Wallets: wallets,
		Total:   len(wallets),
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

// Transfer handles internal wallet-to-wallet transfers
func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	var req TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.Transfer(r.Context(), &req); err != nil {
		h.logger.Errorf("Transfer failed: %v", err)
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "transfer completed",
	})
}

// GetWalletInternal - NO ownership check (for service-to-service calls)
func (h *Handler) GetWalletInternal(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	wallet, err := h.service.GetWallet(r.Context(), walletID)
	if err != nil {
		h.logger.Errorf("Failed to get wallet: %v", err)
		h.respondError(w, http.StatusNotFound, "wallet not found")
		return
	}

	// NO ownership check - internal use only
	h.respondJSON(w, http.StatusOK, WalletResponse{Wallet: wallet})
}