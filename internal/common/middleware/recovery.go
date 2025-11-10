package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/kmassidik/mercuria/internal/common/logger"
)

// Recovery middleware recovers from panics
func Recovery(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Log panic with stack trace
					log.Errorf("PANIC: %v\n%s", err, debug.Stack())

					// Return 500 error
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error":"internal server error"}`))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}