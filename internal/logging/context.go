package logging

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// ContextKey is the type for context keys used in logging.
type ContextKey string

const (
	// RequestIDKey is the context key for request ID.
	RequestIDKey ContextKey = "request_id"
)

// ContextHandler is an slog.Handler that extracts values from context
// and includes them in all log records.
type ContextHandler struct {
	handler slog.Handler
}

// NewContextHandler creates a new context-aware handler that wraps another handler.
func NewContextHandler(handler slog.Handler) *ContextHandler {
	return &ContextHandler{
		handler: handler,
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle adds context attributes to the record and passes it to the underlying handler.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID := ctx.Value(RequestIDKey); reqID != nil {
		r.AddAttrs(slog.Any("request_id", reqID))
	}

	return h.handler.Handle(ctx, r)
}

// WithAttrs returns a new handler with additional attributes.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{
		handler: h.handler.WithAttrs(attrs),
	}
}

// WithGroup returns a new handler with a group.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{
		handler: h.handler.WithGroup(name),
	}
}

// WithRequestID adds a request ID to the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from context.
func GetRequestID(ctx context.Context) (string, bool) {
	reqID, ok := ctx.Value(RequestIDKey).(string)
	return reqID, ok
}

// GenerateRequestID generates a new UUID-based request ID.
func GenerateRequestID() string {
	return uuid.New().String()
}
