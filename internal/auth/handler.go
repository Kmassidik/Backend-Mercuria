package auth

import (
	"encoding/json"
	"net/http"

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

// Register handles user registration
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	authResp, err := h.service.Register(r.Context(), &req)
	if err != nil {
		h.logger.Errorf("Registration failed: %v", err)
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusCreated, authResp)
}

// Login handles user login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	authResp, err := h.service.Login(r.Context(), &req)
	if err != nil {
		h.logger.Errorf("Login failed: %v", err)
		h.respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.respondJSON(w, http.StatusOK, authResp)
}

// Refresh handles token refresh
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	authResp, err := h.service.RefreshAccessToken(r.Context(), req.RefreshToken)
	if err != nil {
		h.logger.Errorf("Token refresh failed: %v", err)
		h.respondError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	h.respondJSON(w, http.StatusOK, authResp)
}

// Me handles getting current user info
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.service.GetCurrentUser(r.Context(), userID)
	if err != nil {
		h.logger.Errorf("Failed to get user: %v", err)
		h.respondError(w, http.StatusNotFound, "user not found")
		return
	}

	h.respondJSON(w, http.StatusOK, user)
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