package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func GenerateCouponCode() (string, error) {
	// Generate 8 random bytes
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating coupon code: %w", err)
	}

	// Convert bytes to a hexadecimal string
	randomHex := hex.EncodeToString(bytes)

	// Generate a timestamp
	timestamp := time.Now().UnixNano()

	// Construct the coupon code
	couponCode := fmt.Sprintf("COUPON-%s-%d", randomHex, timestamp)

	return couponCode, nil
}
