package api

import (
	"context"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rateLimiter implements a simple in-memory rate limiter using fixed window counter algorithm
type rateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     int           // requests per window
	window   time.Duration // time window
}

type visitor struct {
	count    int       // request count in current window
	lastSeen time.Time // last request timestamp
}

// newRateLimiter creates a new rate limiter with the specified rate and window.
// The cleanup goroutine must be started separately by calling startCleanup(ctx).
func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
}

// startCleanup starts the cleanup goroutine that removes stale visitor entries.
// The goroutine will exit when the context is cancelled.
func (rl *rateLimiter) startCleanup(ctx context.Context) {
	go rl.cleanupVisitors(ctx)
}

// cleanupVisitors periodically removes stale visitor entries.
// Exits gracefully when context is cancelled to prevent goroutine leaks.
func (rl *rateLimiter) cleanupVisitors(ctx context.Context) {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			for ip, v := range rl.visitors {
				if time.Since(v.lastSeen) > rl.window {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// isAllowed checks if a request from the given IP is allowed
func (rl *rateLimiter) isAllowed(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	now := time.Now()

	if !exists {
		rl.visitors[ip] = &visitor{count: 1, lastSeen: now}
		return true
	}

	// Reset if window has passed
	if now.Sub(v.lastSeen) > rl.window {
		v.count = 1
		v.lastSeen = now
		return true
	}

	// Check rate limit
	if v.count >= rl.rate {
		return false
	}

	v.count++
	v.lastSeen = now
	return true
}

// rateLimitMiddleware returns a middleware that limits requests per IP.
// This is a method on Server to avoid global state and enable proper cleanup.
func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !s.rateLimiter.isAllowed(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

const (
	// authUserIDKey is the context key for authenticated user ID (package-private)
	authUserIDKey = "api_auth_user_id"
)

// Sentinel errors for authentication and authorization
var (
	ErrUnauthorized        = errors.New("unauthorized")
	ErrAccessDenied        = errors.New("access denied")
	ErrInternalServerError = errors.New("internal server error")
)

// authMiddleware validates the X-User-ID header and stores the user ID in context
// FIXME: This is a simplified auth for development. In production, replace with
// proper authentication using JWT tokens, sessions, or OAuth.
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr := c.GetHeader("X-User-ID")
		if userIDStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing X-User-ID header"})
			return
		}

		// Parse as int64 and validate range to prevent overflow when converting to int32
		// int32 is used because database user IDs are stored as 32-bit integers
		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil || userID < 1 || userID > math.MaxInt32 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid X-User-ID header"})
			return
		}

		c.Set(authUserIDKey, int32(userID))
		c.Next()
	}
}

// getUserIDFromContext extracts the authenticated user ID from the gin context.
// Returns the user ID and nil error on success, or 0 and an error if not found or invalid.
func getUserIDFromContext(ctx *gin.Context) (int32, error) {
	authUserID, exists := ctx.Get(authUserIDKey)
	if !exists {
		return 0, ErrUnauthorized
	}

	userID, ok := authUserID.(int32)
	if !ok {
		log.Printf("ERROR: invalid user ID type in context: %T", authUserID)
		return 0, ErrInternalServerError
	}

	return userID, nil
}

// checkUserAuthorization validates that the authenticated user matches the requested user ID.
// Returns nil if authorized, or an appropriate error with HTTP status code.
// This helper reduces code duplication across handlers that need authorization checks.
func checkUserAuthorization(ctx *gin.Context, requestedUserID int32) error {
	authUserID, err := getUserIDFromContext(ctx)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		} else {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		}
		return err
	}

	if authUserID != requestedUserID {
		ctx.JSON(http.StatusForbidden, errorResponse(ErrAccessDenied))
		return ErrAccessDenied
	}

	return nil
}

// specialTimeMiddleware returns a middleware function that checks if the current time falls within
// the specified time windows for reservation and grab requests.
// This is a method on Server to access timeConfig via dependency injection.
func (s *Server) specialTimeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now().Unix()

		// Check if the current time is within the startReserveTime and endReserveTime range
		if now >= s.timeConfig.startReserveTime.Load() && now < s.timeConfig.endReserveTime.Load() {
			c.Next()
			return
		}

		// Check if the current time is within the startGrabTime and endGrabTime range
		if now >= s.timeConfig.startGrabTime.Load() && now < s.timeConfig.endGrabTime.Load() {
			c.Next()
			return
		}

		// If the current time is not within the specified time ranges, return an error
		c.AbortWithStatusJSON(400, gin.H{"error": "not in special time"})
	}
}
