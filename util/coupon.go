package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"
)

func GenerateCouponCode() string {
	// Generate 8 random bytes
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Handle error
		log.Println("GenerateCouponCode error:", err)
		return ""
	}

	// Convert bytes to a hexadecimal string
	randomHex := hex.EncodeToString(bytes)

	// Generate a timestamp
	timestamp := time.Now().UnixNano()

	// Construct the coupon code
	couponCode := fmt.Sprintf("COUPON-%s-%d", randomHex, timestamp)

	// Store into DB
	return couponCode
}
