package auth

import (
	"testing"
)

func TestGenerateSecureRandomBytes(t *testing.T) {
	bytes, err := GenerateSecureRandomBytes(32)
	if err != nil {
		t.Fatalf("Failed to generate random bytes: %v", err)
	}

	if len(bytes) != 32 {
		t.Errorf("Expected 32 bytes, got %d", len(bytes))
	}

	// Generate another set and verify they're different
	bytes2, err := GenerateSecureRandomBytes(32)
	if err != nil {
		t.Fatalf("Failed to generate second set of random bytes: %v", err)
	}

	// Check if they're different (very unlikely to be the same)
	identical := true
	for i := range bytes {
		if bytes[i] != bytes2[i] {
			identical = false
			break
		}
	}

	if identical {
		t.Error("Two random byte sequences are identical (very unlikely)")
	}
}

func TestGenerateSecureRandomString(t *testing.T) {
	str1, err := GenerateSecureRandomString(32)
	if err != nil {
		t.Fatalf("Failed to generate random string: %v", err)
	}

	if str1 == "" {
		t.Fatal("Generated string is empty")
	}

	// Generate another string and verify they're different
	str2, err := GenerateSecureRandomString(32)
	if err != nil {
		t.Fatalf("Failed to generate second random string: %v", err)
	}

	if str1 == str2 {
		t.Error("Generated strings are identical (very unlikely)")
	}
}

func TestConstantTimeCompare(t *testing.T) {
	a := []byte("test-data-123")
	b := []byte("test-data-123")
	c := []byte("test-data-456")

	// Same data should return true
	if !ConstantTimeCompare(a, b) {
		t.Error("ConstantTimeCompare returned false for identical data")
	}

	// Different data should return false
	if ConstantTimeCompare(a, c) {
		t.Error("ConstantTimeCompare returned true for different data")
	}

	// Different lengths should return false
	d := []byte("test")
	if ConstantTimeCompare(a, d) {
		t.Error("ConstantTimeCompare returned true for different length data")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	a := "test-data-123"
	b := "test-data-123"
	c := "test-data-456"

	// Same strings should return true
	if !ConstantTimeEqual(a, b) {
		t.Error("ConstantTimeEqual returned false for identical strings")
	}

	// Different strings should return false
	if ConstantTimeEqual(a, c) {
		t.Error("ConstantTimeEqual returned true for different strings")
	}

	// Different lengths should return false
	d := "test"
	if ConstantTimeEqual(a, d) {
		t.Error("ConstantTimeEqual returned true for different length strings")
	}
}

func TestGenerateSecureRandomBytes_InvalidSize(t *testing.T) {
	_, err := GenerateSecureRandomBytes(0)
	if err == nil {
		t.Error("Expected error for size 0")
	}

	_, err = GenerateSecureRandomBytes(-1)
	if err == nil {
		t.Error("Expected error for negative size")
	}
}
