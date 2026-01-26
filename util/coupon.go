package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func GenerateCouponCode() (string, error) {
	// Generate 16 random bytes (128-bit entropy) for unpredictable coupon codes
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating coupon code: %w", err)
	}

	// Convert bytes to a hexadecimal string (32 characters)
	randomHex := hex.EncodeToString(bytes)

	// Construct the coupon code without timestamp to prevent enumeration
	couponCode := fmt.Sprintf("COUPON-%s", randomHex)

	return couponCode, nil
}
