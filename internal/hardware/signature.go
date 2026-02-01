package hardware

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	moduleName = "Hardware Signature"
	// HardwareSignatureFilePrefix is the prefix for hardware signature files
	HardwareSignatureFilePrefix = "agent-"
	// HardwareSignatureFileSuffix is the suffix for hardware signature files
	HardwareSignatureFileSuffix = ".jwt"
)

// GetHardwareSignatureFilePath returns the path to the hardware signature file
// Format: /etc/iofog-agent/agent-{uuid}.jwt
func GetHardwareSignatureFilePath() (string, error) {
	cfg := config.GetInstance()
	if cfg.IOFogUUID == "" {
		return "", fmt.Errorf("agent UUID is not set, cannot determine hardware signature file path")
	}

	configDir := utils.ConfigDir
	filename := fmt.Sprintf("%s%s%s", HardwareSignatureFilePrefix, cfg.IOFogUUID, HardwareSignatureFileSuffix)
	return filepath.Join(configDir, filename), nil
}

// ReadHardwareSignature reads the hardware signature from the persistent file
func ReadHardwareSignature() (string, error) {
	filePath, err := GetHardwareSignatureFilePath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read hardware signature file: %w", err)
	}

	signature := string(data)
	logging.LogDebug(moduleName, fmt.Sprintf("Read hardware signature from file: %s (length: %d)", filePath, len(signature)))
	return signature, nil
}

// WriteHardwareSignature writes the hardware signature to the persistent file
func WriteHardwareSignature(signature string) error {
	filePath, err := GetHardwareSignatureFilePath()
	if err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Failed to get hardware signature file path: %v", err), err)
		return fmt.Errorf("failed to get hardware signature file path: %w", err)
	}

	configDir := filepath.Dir(filePath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Failed to create config directory: %s", configDir), err)
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	err = os.WriteFile(filePath, []byte(signature), 0640)
	if err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Failed to write hardware signature file: %s", filePath), err)
		return fmt.Errorf("failed to write hardware signature file: %w", err)
	}

	if fileInfo, err := os.Stat(filePath); err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Hardware signature file was written but cannot be verified: %s", filePath), err)
		return fmt.Errorf("failed to verify hardware signature file creation: %w", err)
	} else {
		logging.LogInfo(moduleName, fmt.Sprintf("Successfully created hardware signature file: %s (size: %d bytes)", filePath, fileInfo.Size()))
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Wrote hardware signature to file: %s (length: %d)", filePath, len(signature)))
	return nil
}

// DeleteHardwareSignature deletes the hardware signature file
func DeleteHardwareSignature() error {
	filePath, err := GetHardwareSignatureFilePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil
	}

	err = os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("failed to delete hardware signature file: %w", err)
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Deleted hardware signature file: %s", filePath))
	return nil
}
