package api

import (
	"time"

	"github.com/gin-gonic/gin"
)

// specialTime is a middleware function that checks if the current time falls within
// the specified time windows for reservation and grab requests
func specialTime(c *gin.Context) {
	now := time.Now().Unix()

	// Check if the current time is within the startReserveTime and endReserveTime range
	if now >= startReserveTime && now < endReserveTime {
		c.Next()
		return
	}

	// Check if the current time is within the startGrabTime and endGrabTime range
	if now >= startGrabTime && now < endGrabTime {
		c.Next()
		return
	}

	// If the current time is not within the specified time ranges, return an error
	c.AbortWithStatusJSON(400, gin.H{"error": "not in special time"})
	return
}
