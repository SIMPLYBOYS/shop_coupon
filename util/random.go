package util

import (
	"math/rand"
	"strings"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

// RandomString generates a random string of length n
func RandomDomain() string {
	var sb strings.Builder

	// Append domain name
	domain := []string{"gmail.com", "yahoo.com", "hotmail.com", "outlook.com"}
	sb.WriteString("@")
	sb.WriteString(domain[rand.Intn(len(domain))])

	return sb.String()
}

func RandomName() string {
	var sb strings.Builder
	k := len(alphabet)

	// Generate random username
	usernameLength := rand.Intn(10) + 5 // Random length between 5 and 14
	for i := 0; i < usernameLength; i++ {
		c := alphabet[rand.Intn(k)]
		sb.WriteByte(c)
	}

	return sb.String()
}

func RandomNumber() int {
	return rand.Intn(10)
}
