package api

import (
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

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

// specialTime is a middleware function that checks if the current time falls within
// the specified time windows for reservation and grab requests
func specialTime(c *gin.Context) {
	now := time.Now().Unix()

	// Check if the current time is within the startReserveTime and endReserveTime range
	if now >= couponTimeConfig.startReserveTime.Load() && now < couponTimeConfig.endReserveTime.Load() {
		c.Next()
		return
	}

	// Check if the current time is within the startGrabTime and endGrabTime range
	if now >= couponTimeConfig.startGrabTime.Load() && now < couponTimeConfig.endGrabTime.Load() {
		c.Next()
		return
	}

	// If the current time is not within the specified time ranges, return an error
	c.AbortWithStatusJSON(400, gin.H{"error": "not in special time"})
	return
}
