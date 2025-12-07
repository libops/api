package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rs/cors"

	"github.com/libops/api/internal/logging"
)

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.statusCode = http.StatusOK
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// AccessLogger logs HTTP requests with method, path, status, and duration.
func AccessLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			written:        false,
		}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log access
		duration := time.Since(start)
		slog.Info(r.Method+" "+r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// RequestIDMiddleware adds a unique request ID to each request's context
// It checks for an existing X-Request-ID header, otherwise generates a new one.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = logging.GenerateRequestID()
		}

		// Add request ID to response headers for client correlation
		w.Header().Set("X-Request-ID", requestID)

		ctx := logging.WithRequestID(r.Context(), requestID)
		ctx = context.WithValue(ctx, "request_id", requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SecurityHeadersMiddleware adds security headers to responses.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware is a placeholder for CSRF protection.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip check for authentication endpoints that don't require a token
		if r.URL.Path == "/auth/token" ||
			strings.HasPrefix(r.URL.Path, "/auth/register/") ||
			strings.HasPrefix(r.URL.Path, "/auth/userpass/") ||
			r.URL.Path == "/auth/login" ||
			r.URL.Path == "/auth/callback" {
			next.ServeHTTP(w, r)
			return
		}

		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// CorsMiddleware creates and applies CORS middleware.
func CorsMiddleware(handler http.Handler, allowedOrigins []string) http.Handler {
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"Connect-Protocol-Version",
			"Connect-Timeout-Ms",
		},
		ExposedHeaders: []string{
			"Connect-Protocol-Version",
			"Connect-Timeout-Ms",
		},
		MaxAge: 7200,
	})
	return corsHandler.Handler(handler)
}
