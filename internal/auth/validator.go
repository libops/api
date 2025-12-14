package auth

import (
	"context"
	"net/http"
)

// JWTValidator defines the interface for a JWT validator middleware.
type JWTValidator interface {
	Middleware(next http.Handler) http.Handler
	ValidateToken(ctx context.Context, tokenString string) (*UserInfo, error)
}
