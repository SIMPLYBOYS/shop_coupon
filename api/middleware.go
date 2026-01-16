package api

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// AuthUserIDKey is the context key for authenticated user ID
	// This is package-private to avoid key collisions with other packages
	AuthUserIDKey = "api_auth_user_id"
)

// authMiddleware validates the X-User-ID header and stores the user ID in context
// In production, this should validate a JWT token or session instead
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

		c.Set(AuthUserIDKey, int32(userID))
		c.Next()
	}
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
