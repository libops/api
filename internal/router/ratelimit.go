package router

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/libops/api/internal/auth"
)

// visitor stores a rate limiter for each visitor and the last time they were seen.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter is a middleware that limits the number of requests per visitor.
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.Mutex
	r        rate.Limit
	b        int
}

// NewRateLimiter creates a new rate limiter middleware.
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		r:        r,
		b:        b,
	}
	go rl.cleanupVisitors()
	return rl
}

// getVisitor returns the rate limiter for the current visitor.
func (rl *RateLimiter) getVisitor(identifier string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[identifier]
	if !exists {
		limiter := rate.NewLimiter(rl.r, rl.b)
		rl.visitors[identifier] = &visitor{limiter, time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupVisitors removes old visitors from the map.
func (rl *RateLimiter) cleanupVisitors() {
	for {
		time.Sleep(1 * time.Minute)

		rl.mu.Lock()
		for identifier, v := range rl.visitors {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(rl.visitors, identifier)
			}
		}
		rl.mu.Unlock()
	}
}

// LimitByIP is a middleware that limits requests by IP address.
// This can be used as either a global limiter or a per-route limiter.
func (rl *RateLimiter) LimitByIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			slog.Error("could not get ip from remote address", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		limiter := rl.getVisitor(ip)
		if !limiter.Allow() {
			slog.Warn("rate limit exceeded for ip", "ip", ip, "path", r.URL.Path, "limit", rl.r, "burst", rl.b)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LimitByUser is a middleware that limits requests by authenticated user.
func (rl *RateLimiter) LimitByUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userInfo, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		limiter := rl.getVisitor(userInfo.EntityID)
		if !limiter.Allow() {
			slog.Warn("rate limit exceeded for user", "user", userInfo.Email)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
