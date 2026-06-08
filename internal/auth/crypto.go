package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
)

// GenerateSecureRandomBytes generates cryptographically secure random bytes
func GenerateSecureRandomBytes(size int) ([]byte, error) {
	if size <= 0 {
		return nil, errors.New("size must be positive")
	}

	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	return bytes, nil
}

// GenerateSecureRandomString generates a cryptographically secure random string
func GenerateSecureRandomString(size int) (string, error) {
	bytes, err := GenerateSecureRandomBytes(size)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ConstantTimeCompare performs a constant-time comparison of two byte slices
// This prevents timing attacks when comparing sensitive data like tokens or keys
func ConstantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// ConstantTimeEqual performs a constant-time comparison of two strings
func ConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// SecureHash generates a secure hash of data (placeholder for future implementation)
// This can be extended to use specific hash functions if needed
func SecureHash(data []byte) ([]byte, error) {
	// For now, this is a placeholder
	// In the future, this could use SHA-256, SHA-512, or other secure hash functions
	// For JWT and token purposes, we typically don't need hashing, but this provides
	// a utility function for other security needs
	return data, nil
}
