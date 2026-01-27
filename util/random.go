package util

import (
	"crypto/rand"
	"math/big"
	"strings"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

// secureRandomInt generates a cryptographically secure random integer in [0, max)
func secureRandomInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		// Fallback: this should never happen in normal circumstances
		// as crypto/rand.Reader is always available on supported platforms
		panic("crypto/rand failed: " + err.Error())
	}
	return int(n.Int64())
}

// RandomDomain returns a random email domain for testing purposes
func RandomDomain() string {
	var sb strings.Builder

	// Append domain name
	domain := []string{"gmail.com", "yahoo.com", "hotmail.com", "outlook.com"}
	sb.WriteString("@")
	sb.WriteString(domain[secureRandomInt(len(domain))])

	return sb.String()
}

// RandomName generates a random username for testing purposes
func RandomName() string {
	var sb strings.Builder
	k := len(alphabet)

	// Generate random username
	usernameLength := secureRandomInt(10) + 5 // Random length between 5 and 14
	for i := 0; i < usernameLength; i++ {
		c := alphabet[secureRandomInt(k)]
		sb.WriteByte(c)
	}

	return sb.String()
}

// RandomNumber generates a cryptographically secure random number in [0, 10)
func RandomNumber() int {
	return secureRandomInt(10)
}
