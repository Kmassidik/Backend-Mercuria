package analytics

import (
	"net/http"
)

func SetupRoutes(mux *http.ServeMux, handler *Handler) {
	// Health checks
	mux.HandleFunc("GET /health", handler.HealthCheck)
	mux.HandleFunc("GET /ready", handler.ReadinessCheck)

	// Analytics API v1
	mux.HandleFunc("GET /api/v1/analytics/daily", handler.GetDailyMetrics)
	mux.HandleFunc("GET /api/v1/analytics/hourly", handler.GetHourlyMetrics)
	mux.HandleFunc("GET /api/v1/analytics/summary", handler.GetMetricsSummary)
	
	// User analytics
	mux.HandleFunc("GET /api/v1/analytics/users/{user_id}", handler.GetUserAnalytics)
	mux.HandleFunc("GET /api/v1/analytics/users/{user_id}/snapshots", handler.GetUserSnapshots)
}