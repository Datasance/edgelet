package fieldagent

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

// CacheFile represents a cached file with checksum and timestamp
type CacheFile struct {
	Checksum  string          `json:"checksum"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// checksum computes SHA1 checksum of data
func checksum(data string) string {
	base64Data := base64.StdEncoding.EncodeToString([]byte(data))
	hash := sha1.Sum([]byte(base64Data))
	return fmt.Sprintf("%x", hash)
}

// getCachePath returns the path to the cache directory
func getCachePath() string {
	return utils.ConfigDir
}

// SaveFile saves data to a cache file with checksum
func SaveFile(data interface{}, filename string) error {
	logging.LogDebug("Field Agent Cache", fmt.Sprintf("Start save file: %s", filename))

	// Marshal data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Compute checksum
	dataStr := string(jsonData)
	checksum := checksum(dataStr)

	// Create cache file structure
	cacheFile := CacheFile{
		Checksum:  checksum,
		Timestamp: time.Now().UnixMilli(),
		Data:      json.RawMessage(jsonData),
	}

	// Marshal cache file
	cacheData, err := json.Marshal(cacheFile)
	if err != nil {
		return fmt.Errorf("failed to marshal cache file: %w", err)
	}

	// Ensure directory exists
	cachePath := getCachePath()
	if err := os.MkdirAll(cachePath, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Write file
	filePath := filepath.Join(cachePath, filename)
	if err := os.WriteFile(filePath, cacheData, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	logging.LogDebug("Field Agent Cache", fmt.Sprintf("Finished save file: %s", filename))
	return nil
}

// ReadFile reads data from a cache file and validates checksum
func ReadFile(filename string) (json.RawMessage, int64, error) {
	logging.LogDebug("Field Agent Cache", fmt.Sprintf("Start read file: %s", filename))

	filePath := filepath.Join(getCachePath(), filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, 0, nil
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read cache file: %w", err)
	}

	// Unmarshal cache file
	var cacheFile CacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal cache file: %w", err)
	}

	// Validate checksum
	dataStr := string(cacheFile.Data)
	computedChecksum := checksum(dataStr)
	if computedChecksum != cacheFile.Checksum {
		logging.LogWarn("Field Agent Cache", fmt.Sprintf("Checksum mismatch for file: %s", filename))
		return nil, 0, fmt.Errorf("checksum mismatch")
	}

	logging.LogDebug("Field Agent Cache", fmt.Sprintf("Finished read file: %s", filename))
	return cacheFile.Data, cacheFile.Timestamp, nil
}

// ReadFileAsArray reads a cached file as a JSON array
func ReadFileAsArray(filename string) ([]map[string]interface{}, int64, error) {
	data, timestamp, err := ReadFile(filename)
	if err != nil || data == nil {
		return nil, timestamp, err
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, timestamp, fmt.Errorf("failed to unmarshal array: %w", err)
	}

	return result, timestamp, nil
}
