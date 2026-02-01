package auth

import (
	"os"
	"testing"

	"github.com/eclipse-iofog/agent-go/internal/utils"
)

func TestGenerateToken(t *testing.T) {
	token1, err := GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token1 == "" {
		t.Fatal("Generated token is empty")
	}

	// Generate another token and verify they're different
	token2, err := GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate second token: %v", err)
	}

	if token1 == token2 {
		t.Error("Generated tokens are identical (very unlikely)")
	}
}

func TestLocalTokenManager_SaveAndLoad(t *testing.T) {
	// Create temporary file path
	tmpFile, err := os.CreateTemp("", "test-token-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Override the token path for testing
	originalPath := utils.LocalAPITokenPath
	utils.LocalAPITokenPath = tmpFile.Name()
	defer func() {
		utils.LocalAPITokenPath = originalPath
	}()

	manager := GetLocalTokenManager()
	manager.Reset()

	// Generate and save token
	token, err := manager.GenerateAndSaveToken()
	if err != nil {
		t.Fatalf("Failed to generate and save token: %v", err)
	}

	if token == "" {
		t.Fatal("Generated token is empty")
	}

	// Reset manager to force reload
	manager.Reset()

	// Load token
	loadedToken, err := manager.LoadToken()
	if err != nil {
		t.Fatalf("Failed to load token: %v", err)
	}

	if loadedToken != token {
		t.Errorf("Loaded token doesn't match saved token. Expected: %s, Got: %s", token, loadedToken)
	}
}

func TestLocalTokenManager_ValidateToken(t *testing.T) {
	// Create temporary file path
	tmpFile, err := os.CreateTemp("", "test-token-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Override the token path for testing
	originalPath := utils.LocalAPITokenPath
	utils.LocalAPITokenPath = tmpFile.Name()
	defer func() {
		utils.LocalAPITokenPath = originalPath
	}()

	manager := GetLocalTokenManager()
	manager.Reset()

	// Generate and save token
	token, err := manager.GenerateAndSaveToken()
	if err != nil {
		t.Fatalf("Failed to generate and save token: %v", err)
	}

	// Validate correct token
	if !manager.ValidateToken(token) {
		t.Error("Valid token was rejected")
	}

	// Validate incorrect token
	if manager.ValidateToken("invalid-token") {
		t.Error("Invalid token was accepted")
	}

	// Validate empty token
	if manager.ValidateToken("") {
		t.Error("Empty token was accepted")
	}
}

func TestLocalTokenManager_LoadToken_FileNotExists(t *testing.T) {
	// Create temporary file path that doesn't exist
	tmpFile, err := os.CreateTemp("", "test-token-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	os.Remove(tmpFile.Name()) // Remove it so it doesn't exist

	// Override the token path for testing
	originalPath := utils.LocalAPITokenPath
	utils.LocalAPITokenPath = tmpFile.Name()
	defer func() {
		utils.LocalAPITokenPath = originalPath
	}()

	// Get a fresh manager instance by resetting
	manager := GetLocalTokenManager()
	manager.Reset()

	// Try to load token from non-existent file
	_, err = manager.LoadToken()
	if err == nil {
		t.Error("Expected error when loading token from non-existent file, got nil")
	}
	
	// Verify the error message contains expected text
	if err != nil && err.Error() == "" {
		t.Error("Error message is empty")
	}
}
