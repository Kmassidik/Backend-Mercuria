package analytics

import (
	"net/http"

	"github.com/kmassidik/mercuria/internal/common/middleware"
)

// SetupRoutes - PUBLIC API (HTTPS + JWT for external clients)
func SetupRoutes(mux *http.ServeMux, handler *Handler, jwtSecret string) {
	protected := middleware.JWTAuth(jwtSecret)

	// Public health checks (no auth)
	mux.HandleFunc("GET /health", handler.HealthCheck)
	mux.HandleFunc("GET /ready", handler.ReadinessCheck)

	// Protected - system-wide metrics (requires JWT)
	mux.Handle("GET /api/v1/analytics/daily", protected(http.HandlerFunc(handler.GetDailyMetrics)))
	mux.Handle("GET /api/v1/analytics/hourly", protected(http.HandlerFunc(handler.GetHourlyMetrics)))
	mux.Handle("GET /api/v1/analytics/summary", protected(http.HandlerFunc(handler.GetMetricsSummary)))
	
	// Protected - user-specific analytics (NO {user_id} in path - extracted from JWT)
	mux.Handle("GET /api/v1/analytics/me", protected(http.HandlerFunc(handler.GetUserAnalytics)))
	mux.Handle("GET /api/v1/analytics/me/snapshots", protected(http.HandlerFunc(handler.GetUserSnapshots)))
}

// SetupInternalRoutes - INTERNAL API (mTLS only, NO JWT needed)
// Used for service-to-service communication
func SetupInternalRoutes(mux *http.ServeMux, handler *Handler) {
	// Internal health check
	mux.HandleFunc("GET /health", handler.HealthCheck)

	// Internal analytics queries (mTLS authenticated, no JWT needed)
	// These can be called by other services for cross-service analytics
	mux.HandleFunc("GET /api/v1/internal/analytics/metrics", handler.GetDailyMetrics)
	mux.HandleFunc("GET /api/v1/internal/analytics/summary", handler.GetMetricsSummary)
}