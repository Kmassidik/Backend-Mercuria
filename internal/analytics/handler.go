package analytics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kmassidik/mercuria/internal/common/middleware"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// SuccessResponse represents a success response
type SuccessResponse struct {
	Data    interface{} `json:"data"`
	Message string      `json:"message,omitempty"`
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response
func writeError(w http.ResponseWriter, status int, err string, message string) {
	writeJSON(w, status, ErrorResponse{
		Error:   err,
		Message: message,
	})
}

// GetDailyMetrics handles GET /api/v1/analytics/daily
func (h *Handler) GetDailyMetrics(w http.ResponseWriter, r *http.Request) {
	// Public API requires JWT, internal mTLS calls are also allowed
	// No explicit check needed - middleware handles authentication

	// Parse query parameters
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")

	if startDateStr == "" || endDateStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_parameters", "start_date and end_date are required")
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_date_format", "start_date must be in YYYY-MM-DD format")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_date_format", "end_date must be in YYYY-MM-DD format")
		return
	}

	// Validate date range
	if endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "invalid_date_range", "end_date must be after start_date")
		return
	}

	// Limit to 90 days
	if endDate.Sub(startDate) > 90*24*time.Hour {
		writeError(w, http.StatusBadRequest, "date_range_too_large", "date range cannot exceed 90 days")
		return
	}

	// Fetch metrics
	metrics, err := h.service.GetDailyMetrics(r.Context(), startDate, endDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch daily metrics")
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{
		Data: metrics,
	})
}

// GetHourlyMetrics handles GET /api/v1/analytics/hourly
func (h *Handler) GetHourlyMetrics(w http.ResponseWriter, r *http.Request) {
	// Public API requires JWT, internal mTLS calls are also allowed
	// No explicit check needed - middleware handles authentication

	// Parse query parameters
	startTimeStr := r.URL.Query().Get("start_time")
	endTimeStr := r.URL.Query().Get("end_time")

	if startTimeStr == "" || endTimeStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_parameters", "start_time and end_time are required")
		return
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_time_format", "start_time must be in RFC3339 format")
		return
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_time_format", "end_time must be in RFC3339 format")
		return
	}

	// Validate time range
	if endTime.Before(startTime) {
		writeError(w, http.StatusBadRequest, "invalid_time_range", "end_time must be after start_time")
		return
	}

	// Limit to 7 days
	if endTime.Sub(startTime) > 7*24*time.Hour {
		writeError(w, http.StatusBadRequest, "time_range_too_large", "time range cannot exceed 7 days")
		return
	}

	// Fetch metrics
	metrics, err := h.service.GetHourlyMetrics(r.Context(), startTime, endTime)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch hourly metrics")
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{
		Data: metrics,
	})
}

// GetMetricsSummary handles GET /api/v1/analytics/summary
func (h *Handler) GetMetricsSummary(w http.ResponseWriter, r *http.Request) {
	// Public API requires JWT, internal mTLS calls are also allowed
	// No explicit check needed - middleware handles authentication

	// Parse query parameters
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")
	period := r.URL.Query().Get("period")

	if startDateStr == "" || endDateStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_parameters", "start_date and end_date are required")
		return
	}

	if period == "" {
		period = "daily" // Default to daily
	}

	if period != "daily" && period != "hourly" {
		writeError(w, http.StatusBadRequest, "invalid_period", "period must be 'daily' or 'hourly'")
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_date_format", "start_date must be in YYYY-MM-DD format")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_date_format", "end_date must be in YYYY-MM-DD format")
		return
	}

	// Validate date range
	if endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "invalid_date_range", "end_date must be after start_date")
		return
	}

	// Fetch summary
	summary, err := h.service.GetMetricsSummary(r.Context(), startDate, endDate, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch metrics summary")
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{
		Data: summary,
	})
}

// GetUserAnalytics handles GET /api/v1/analytics/me
// Gets analytics for the authenticated user across all their wallets
func (h *Handler) GetUserAnalytics(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Parse dates with defaults (last 30 days)
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")

	endDate := time.Now().Truncate(24 * time.Hour)
	startDate := endDate.AddDate(0, 0, -30)

	if startDateStr != "" {
		var err error
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date_format", "start_date must be in YYYY-MM-DD format")
			return
		}
	}

	if endDateStr != "" {
		var err error
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date_format", "end_date must be in YYYY-MM-DD format")
			return
		}
	}

	// Validate date range
	if endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "invalid_date_range", "end_date must be after start_date")
		return
	}

	// Limit to 90 days
	if endDate.Sub(startDate) > 90*24*time.Hour {
		writeError(w, http.StatusBadRequest, "date_range_too_large", "date range cannot exceed 90 days")
		return
	}

	// Get user's wallet IDs from Wallet Service
	walletClient := NewWalletClient()
	authToken := r.Header.Get("Authorization")
	
	walletIDs, err := walletClient.GetUserWalletIDs(r.Context(), userID, authToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "wallet_service_error", fmt.Sprintf("Failed to fetch user wallets: %v", err))
		return
	}

	// If user has no wallets, return empty analytics
	if len(walletIDs) == 0 {
		writeJSON(w, http.StatusOK, SuccessResponse{
			Data: &UserAnalyticsResponse{
				UserID:           userID,
				Period:           fmt.Sprintf("%s to %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02")),
				TotalSent:        0,
				TotalReceived:    0,
				NetAmount:        0,
				TransactionCount: 0,
				TotalFeesPaid:    0,
			},
		})
		return
	}

	// Fetch analytics across all user's wallets
	analytics, err := h.service.GetUserAnalyticsByWallets(r.Context(), walletIDs, startDate, endDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch user analytics")
		return
	}

	// Set the user ID in the response
	analytics.UserID = userID

	writeJSON(w, http.StatusOK, SuccessResponse{
		Data: analytics,
	})
}

// GetUserSnapshots handles GET /api/v1/analytics/me/snapshots
// Gets daily snapshots for the authenticated user across all their wallets
func (h *Handler) GetUserSnapshots(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Parse dates with defaults (last 30 days)
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")

	endDate := time.Now().Truncate(24 * time.Hour)
	startDate := endDate.AddDate(0, 0, -30)

	if startDateStr != "" {
		var err error
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date_format", "start_date must be in YYYY-MM-DD format")
			return
		}
	}

	if endDateStr != "" {
		var err error
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date_format", "end_date must be in YYYY-MM-DD format")
			return
		}
	}

	// Validate date range
	if endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "invalid_date_range", "end_date must be after start_date")
		return
	}

	// Limit to 90 days
	if endDate.Sub(startDate) > 90*24*time.Hour {
		writeError(w, http.StatusBadRequest, "date_range_too_large", "date range cannot exceed 90 days")
		return
	}

	// Get user's wallet IDs from Wallet Service
	walletClient := NewWalletClient()
	authToken := r.Header.Get("Authorization")
	
	walletIDs, err := walletClient.GetUserWalletIDs(r.Context(), userID, authToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "wallet_service_error", fmt.Sprintf("Failed to fetch user wallets: %v", err))
		return
	}

	// If user has no wallets, return empty snapshots
	if len(walletIDs) == 0 {
		writeJSON(w, http.StatusOK, SuccessResponse{
			Data: []UserSnapshot{},
		})
		return
	}

	// Fetch snapshots across all user's wallets
	snapshots, err := h.service.GetUserSnapshotsByWallets(r.Context(), walletIDs, startDate, endDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch user snapshots")
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{
		Data: snapshots,
	})
}

// HealthCheck handles GET /health
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"service":   "analytics",
		"timestamp": time.Now().UTC(),
	})
}

// ReadinessCheck handles GET /ready
func (h *Handler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ready",
		"service": "analytics",
	})
}