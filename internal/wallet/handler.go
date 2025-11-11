package wallet

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/middleware"
)

type Handler struct {
	service *Service
	logger  *logger.Logger
}

func NewHandler(service *Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// CreateWallet handles wallet creation
func (h *Handler) CreateWallet(w http.ResponseWriter, r *http.Request) {
	// Get user ID from JWT context
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

	// Set user ID from JWT token
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
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	// Get user ID from JWT
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

	// Verify wallet belongs to user
	if wallet.UserID != userID {
		h.respondError(w, http.StatusForbidden, "access denied")
		return
	}

	h.respondJSON(w, http.StatusOK, WalletResponse{Wallet: wallet})
}

// Deposit handles deposit requests
func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	// Get user ID from JWT
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

	// Verify wallet ownership
	wallet, err := h.service.GetWallet(r.Context(), walletID)
	if err != nil {
		h.respondError(w, http.StatusNotFound, "wallet not found")
		return
	}

	if wallet.UserID != userID {
		h.respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Process deposit
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
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	// Get user ID from JWT
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

	// Verify wallet ownership
	wallet, err := h.service.GetWallet(r.Context(), walletID)
	if err != nil {
		h.respondError(w, http.StatusNotFound, "wallet not found")
		return
	}

	if wallet.UserID != userID {
		h.respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Process withdrawal
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
	walletID := r.PathValue("id")
	if walletID == "" {
		h.respondError(w, http.StatusBadRequest, "wallet ID is required")
		return
	}

	// Get user ID from JWT
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Verify wallet ownership
	wallet, err := h.service.GetWallet(r.Context(), walletID)
	if err != nil {
		h.respondError(w, http.StatusNotFound, "wallet not found")
		return
	}

	if wallet.UserID != userID {
		h.respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Parse pagination params
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

	// Get events
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

// Helper methods
func (h *Handler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, ErrorResponse{Error: message})
}