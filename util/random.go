package util

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

// secureRandomInt generates a cryptographically secure random integer in [0, max)
func secureRandomInt(max int) (int, error) {
	if max <= 0 {
		return 0, fmt.Errorf("max must be positive, got %d", max)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, fmt.Errorf("generating secure random int: %w", err)
	}
	return int(n.Int64()), nil
}

// RandomDomain returns a random email domain for testing purposes
func RandomDomain() (string, error) {
	domain := []string{"gmail.com", "yahoo.com", "hotmail.com", "outlook.com"}
	idx, err := secureRandomInt(len(domain))
	if err != nil {
		return "", fmt.Errorf("generating random domain: %w", err)
	}
	return "@" + domain[idx], nil
}

// RandomName generates a random username for testing purposes
func RandomName() (string, error) {
	var sb strings.Builder
	k := len(alphabet)

	// Generate random username
	usernameLength, err := secureRandomInt(10)
	if err != nil {
		return "", fmt.Errorf("generating random name length: %w", err)
	}
	usernameLength += 5 // Random length between 5 and 14

	for i := 0; i < usernameLength; i++ {
		idx, err := secureRandomInt(k)
		if err != nil {
			return "", fmt.Errorf("generating random name character at index %d: %w", i, err)
		}
		sb.WriteByte(alphabet[idx])
	}

	return sb.String(), nil
}

// RandomNumber generates a cryptographically secure random number in [0, 10)
func RandomNumber() (int, error) {
	n, err := secureRandomInt(10)
	if err != nil {
		return 0, fmt.Errorf("generating random number: %w", err)
	}
	return n, nil
}
