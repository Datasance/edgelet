package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	edgeletAPITokenModuleName = "Edgelet API Token" // #nosec G101 -- logging module name, not a credential
	tokenSize                 = 32                  // 32 bytes = 256 bits
)

// LocalTokenManager manages local API tokens
type LocalTokenManager struct {
	mu    sync.RWMutex
	token string
	path  string
}

var (
	localTokenInstance *LocalTokenManager
	localTokenOnce     sync.Once
)

// GetLocalTokenManager returns the singleton local token manager instance
func GetLocalTokenManager() *LocalTokenManager {
	localTokenOnce.Do(func() {
		localTokenInstance = &LocalTokenManager{
			path: utils.EdgeletAPITokenPath,
		}
	})
	// Update path in case it changed (useful for testing and dev environment)
	// Recalculate path to ensure it uses current SNAP_COMMON value
	localTokenInstance.mu.Lock()
	// Recalculate ConfigDir in case SNAP_COMMON changed (for dev environment)
	configDir := utils.GetConfigDir()
	localTokenInstance.path = configDir + "edgelet-api"
	localTokenInstance.mu.Unlock()
	return localTokenInstance
}

// GenerateToken generates a secure random token
func GenerateToken() (string, error) {
	token := make([]byte, tokenSize)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	// Encode to base64 URL-safe format
	return base64.URLEncoding.EncodeToString(token), nil
}

// LoadToken loads the token from the file
func (ltm *LocalTokenManager) LoadToken() (string, error) {
	ltm.mu.RLock()
	defer ltm.mu.RUnlock()

	// Return cached token if available
	if ltm.token != "" {
		return ltm.token, nil
	}

	// Read token from file
	data, err := os.ReadFile(ltm.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("local API access token is missing, try to re-install Agent: %w", err)
		}
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	// Token is stored as a single line, trim whitespace
	token := string(data)
	if len(token) == 0 {
		return "", errors.New("token file is empty")
	}

	// Cache the token
	ltm.token = token

	return token, nil
}

// SaveToken saves a token to the file
func (ltm *LocalTokenManager) SaveToken(token string) error {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()

	// Write token to file with secure permissions (0600)
	err := os.WriteFile(ltm.path, []byte(token), 0600)
	if err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	// Cache the token
	ltm.token = token

	logging.LogDebug(edgeletAPITokenModuleName, "Token saved successfully")
	return nil
}

// GenerateAndSaveToken generates a new token and saves it to the file
func (ltm *LocalTokenManager) GenerateAndSaveToken() (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}

	if err := ltm.SaveToken(token); err != nil {
		return "", err
	}

	return token, nil
}

// ValidateToken validates a token against the stored token using constant-time comparison
// Matches Java: accessToken.equalsIgnoreCase(validAccessToken)
func (ltm *LocalTokenManager) ValidateToken(providedToken string) bool {
	storedToken, err := ltm.LoadToken()
	if err != nil {
		logging.LogError(edgeletAPITokenModuleName, fmt.Sprintf("Failed to load token for validation from path: %s", ltm.path), err)
		return false
	}

	// Java uses equalsIgnoreCase, so we should do case-insensitive comparison
	// But for security, we'll use constant-time comparison with normalized case
	// Normalize both tokens to lowercase for case-insensitive comparison (matching Java)
	providedTokenLower := strings.ToLower(strings.TrimSpace(providedToken))
	storedTokenLower := strings.ToLower(strings.TrimSpace(storedToken))

	// Use constant-time comparison to prevent timing attacks
	return constantTimeCompare(storedTokenLower, providedTokenLower)
}

// constantTimeCompare performs a constant-time comparison of two strings
// This prevents timing attacks when comparing tokens
func constantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Reset clears the cached token (forces reload from file)
func (ltm *LocalTokenManager) Reset() {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()
	ltm.token = ""
	// Update path in case it changed
	ltm.path = utils.EdgeletAPITokenPath
}
