package analytics

import (
	"encoding/json"
	"net/http"
	"time"
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

// GetUserAnalytics handles GET /api/v1/analytics/users/{user_id}
func (h *Handler) GetUserAnalytics(w http.ResponseWriter, r *http.Request) {
	// Extract user_id from URL path
	userID := r.PathValue("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "invalid_parameters", "user_id is required")
		return
	}

	// Parse query parameters
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")

	// Default to last 30 days if not specified
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

	// Fetch user analytics
	analytics, err := h.service.GetUserAnalytics(r.Context(), userID, startDate, endDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch user analytics")
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{
		Data: analytics,
	})
}

// GetUserSnapshots handles GET /api/v1/analytics/users/{user_id}/snapshots
func (h *Handler) GetUserSnapshots(w http.ResponseWriter, r *http.Request) {
	// Extract user_id from URL path
	userID := r.PathValue("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "invalid_parameters", "user_id is required")
		return
	}

	// Parse query parameters
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")

	// Default to last 30 days if not specified
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

	// Fetch user snapshots
	snapshots, err := h.service.GetUserSnapshots(r.Context(), userID, startDate, endDate)
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
	// Could add database connectivity check here
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ready",
		"service": "analytics",
	})
}