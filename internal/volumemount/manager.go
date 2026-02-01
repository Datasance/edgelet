package volumemount

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/statusreporter"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	moduleName        = "VolumeMountManager"
	volumesDir        = "volumes"
	indexFile         = "index.json"
	secretsDir        = "secrets"
	configMapsDir     = "configMaps"
	microservicesDir  = "microservices"
	maxVersionHistory = 1 // Keep only current version for simplicity
	dataSymlink       = "..data"
)

// VolumeMountType represents the type of volume mount
type VolumeMountType string

const (
	VolumeMountTypeSecret    VolumeMountType = "SECRET"
	VolumeMountTypeConfigMap VolumeMountType = "CONFIGMAP"
	VolumeMountTypeMicroservice VolumeMountType = "MICROSERVICE"
)

// VolumeMountManager manages volume mounts for microservices
type VolumeMountManager struct {
	baseDirectory string
	indexData     map[string]interface{}
	indexLock     sync.RWMutex
	typeCache     map[string]VolumeMountType
	typeCacheLock sync.RWMutex
}

var (
	instance *VolumeMountManager
	once     sync.Once
)

// GetInstance returns the singleton VolumeMountManager instance
func GetInstance() *VolumeMountManager {
	once.Do(func() {
		cfg := config.GetInstance()
		baseDir := filepath.Join(cfg.DiskDirectory, volumesDir)
		instance = &VolumeMountManager{
			baseDirectory: baseDir,
			indexData:       make(map[string]interface{}),
			typeCache:       make(map[string]VolumeMountType),
		}
		instance.init()
	})
	return instance
}

// init initializes the volume mount manager
func (vmm *VolumeMountManager) init() {
	logging.LogDebug(moduleName, "Initializing volume mount manager")
	
	// Create volumes directory if it doesn't exist
	if err := os.MkdirAll(vmm.baseDirectory, 0755); err != nil {
		logging.LogError(moduleName, "Error creating volumes directory", err)
		return
	}
	
	// Load or create index file
	vmm.loadIndex()
	
	// Rebuild type cache after loading index
	vmm.rebuildTypeCache()
	
	logging.LogInfo(moduleName, "Volume mount manager initialized successfully")
}

// loadIndex loads the index file or creates it if it doesn't exist
func (vmm *VolumeMountManager) loadIndex() {
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()
	
	indexPath := filepath.Join(vmm.baseDirectory, indexFile)
	backupPath := filepath.Join(vmm.baseDirectory, indexFile+".bak")
	
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		logging.LogDebug(moduleName, "Creating new index file")
		vmm.indexData = make(map[string]interface{})
		vmm.saveIndex()
		return
	}
	
	logging.LogDebug(moduleName, "Loading existing index file")
	
	// Read index file
	data, err := os.ReadFile(indexPath)
	if err != nil {
		logging.LogError(moduleName, "Error reading index file", err)
		vmm.restoreIndexFromBackup(backupPath)
		return
	}
	
	// Parse JSON
	var fileData map[string]interface{}
	if err := json.Unmarshal(data, &fileData); err != nil {
		logging.LogError(moduleName, "Error parsing index file", err)
		vmm.restoreIndexFromBackup(backupPath)
		return
	}
	
	// Verify checksum
	storedChecksum, ok1 := fileData["checksum"].(string)
	dataObj, ok2 := fileData["data"].(map[string]interface{})
	if !ok1 || !ok2 {
		logging.LogError(moduleName, "Invalid index file format", fmt.Errorf("missing checksum or data"))
		vmm.restoreIndexFromBackup(backupPath)
		return
	}
	
	// Compute checksum
	dataBytes, _ := json.Marshal(dataObj)
	computedChecksum := vmm.checksum(string(dataBytes))
	if computedChecksum != storedChecksum {
		logging.LogError(moduleName, "Index file checksum verification failed", 
			fmt.Errorf("index file may have been tampered with"))
		vmm.restoreIndexFromBackup(backupPath)
		return
	}
	
	vmm.indexData = dataObj
	vmm.migrateIndexFormatIfNeeded()
}

// restoreIndexFromBackup restores index from backup file
func (vmm *VolumeMountManager) restoreIndexFromBackup(backupPath string) {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		logging.LogInfo(moduleName, "No backup available, initializing empty index")
		vmm.indexData = make(map[string]interface{})
		return
	}
	
	logging.LogInfo(moduleName, "Restoring index from backup")
	data, err := os.ReadFile(backupPath)
	if err != nil {
		logging.LogError(moduleName, "Error reading backup file", err)
		vmm.indexData = make(map[string]interface{})
		return
	}
	
	var fileData map[string]interface{}
	if err := json.Unmarshal(data, &fileData); err != nil {
		logging.LogError(moduleName, "Error parsing backup file", err)
		vmm.indexData = make(map[string]interface{})
		return
	}
	
	if dataObj, ok := fileData["data"].(map[string]interface{}); ok {
		vmm.indexData = dataObj
		vmm.saveIndex()
	} else {
		vmm.indexData = make(map[string]interface{})
	}
}

// migrateIndexFormatIfNeeded migrates old index format to new format
func (vmm *VolumeMountManager) migrateIndexFormatIfNeeded() {
	needsMigration := false
	newIndexData := make(map[string]interface{})
	
	for uuid, mountDataValue := range vmm.indexData {
		mountData, ok := mountDataValue.(map[string]interface{})
		if !ok {
			newIndexData[uuid] = mountDataValue
			continue
		}
		
		newMountData := make(map[string]interface{})
		for k, v := range mountData {
			newMountData[k] = v
		}
		
		// Add type field if missing
		if _, exists := mountData["type"]; !exists {
			newMountData["type"] = "secret"
			needsMigration = true
		}
		
		// Add microservices array if missing
		if _, exists := mountData["microservices"]; !exists {
			newMountData["microservices"] = []interface{}{}
			needsMigration = true
		}
		
		newIndexData[uuid] = newMountData
	}
	
	if needsMigration {
		logging.LogInfo(moduleName, "Migrating index format to new schema")
		vmm.indexData = newIndexData
		vmm.saveIndex()
	}
}

// saveIndex saves the index file atomically with backup
func (vmm *VolumeMountManager) saveIndex() {
	indexPath := filepath.Join(vmm.baseDirectory, indexFile)
	tempPath := filepath.Join(vmm.baseDirectory, indexFile+".tmp")
	backupPath := filepath.Join(vmm.baseDirectory, indexFile+".bak")
	
	logging.LogDebug(moduleName, "Saving index file")
	
	// Create wrapper object with checksum and timestamp
	dataBytes, _ := json.Marshal(vmm.indexData)
	checksum := vmm.checksum(string(dataBytes))
	
	wrapper := map[string]interface{}{
		"checksum":  checksum,
		"timestamp": time.Now().UnixMilli(),
		"data":      vmm.indexData,
	}
	
	// Write to temp file
	wrapperBytes, _ := json.MarshalIndent(wrapper, "", "  ")
	if err := os.WriteFile(tempPath, wrapperBytes, 0644); err != nil {
		logging.LogError(moduleName, "Error writing temp index file", err)
		vmm.restoreIndexFromBackup(backupPath)
		return
	}
	
	// Create backup of current index if it exists and is different
	if _, err := os.Stat(indexPath); err == nil {
		data, readErr := os.ReadFile(indexPath)
		if readErr == nil {
			var currentFileData map[string]interface{}
			if json.Unmarshal(data, &currentFileData) == nil {
				if currentChecksum, ok := currentFileData["checksum"].(string); ok {
					if currentChecksum != checksum {
						// Copy to backup
						if err := copyFile(indexPath, backupPath); err != nil {
							logging.LogWarn(moduleName, fmt.Sprintf("Error creating backup: %v", err))
						}
					}
				}
			}
		}
	}
	
	// Atomic move temp to final location
	if err := os.Rename(tempPath, indexPath); err != nil {
		logging.LogError(moduleName, "Error moving temp index file", err)
		vmm.restoreIndexFromBackup(backupPath)
		return
	}
	
	// Update volume mount status
	activeMounts := int64(len(vmm.indexData))
	statusreporter.GetInstance().UpdateVolumeMountManagerStatus(activeMounts, time.Now().UnixMilli())
}

// checksum computes SHA256 checksum of a string
func (vmm *VolumeMountManager) checksum(data string) string {
	h := sha256.New()
	h.Write([]byte(data))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// rebuildTypeCache rebuilds the type cache from index data
// Acquires indexLock - use rebuildTypeCacheUnsafe if lock is already held
func (vmm *VolumeMountManager) rebuildTypeCache() {
	vmm.indexLock.RLock()
	defer vmm.indexLock.RUnlock()
	vmm.rebuildTypeCacheUnsafe()
}

// rebuildTypeCacheUnsafe rebuilds the type cache without acquiring indexLock
// Caller must hold indexLock (read or write)
func (vmm *VolumeMountManager) rebuildTypeCacheUnsafe() {
	vmm.typeCacheLock.Lock()
	defer vmm.typeCacheLock.Unlock()
	
	vmm.typeCache = make(map[string]VolumeMountType)
	
	for _, mountDataValue := range vmm.indexData {
		mountData, ok := mountDataValue.(map[string]interface{})
		if !ok {
			continue
		}
		
		name, _ := mountData["name"].(string)
		typeStr, _ := mountData["type"].(string)
		
		if name != "" {
			if typeStr == "secret" {
				vmm.typeCache[name] = VolumeMountTypeSecret
			} else {
				vmm.typeCache[name] = VolumeMountTypeConfigMap
			}
		}
	}
}

// GetVolumeMountType gets the volume mount type by name (from cache)
func (vmm *VolumeMountManager) GetVolumeMountType(volumeName string) VolumeMountType {
	vmm.typeCacheLock.RLock()
	defer vmm.typeCacheLock.RUnlock()
	return vmm.typeCache[volumeName]
}

// ProcessVolumeMountChanges processes volume mount changes from controller
// Matches Java: processVolumeMountChanges() - handles errors gracefully
func (vmm *VolumeMountManager) ProcessVolumeMountChanges(volumeMounts []interface{}) {
	// Use defer/recover to catch any panics (matching Java try-catch)
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(moduleName, fmt.Sprintf("Panic in ProcessVolumeMountChanges: %v", r), fmt.Errorf("%v", r))
		}
	}()
	
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()
	
	logging.LogInfo(moduleName, "Processing volume mount changes")
	
	// Get existing volume mount UUIDs
	existingUuids := make(map[string]bool)
	for uuid := range vmm.indexData {
		existingUuids[uuid] = true
	}
	
	// Get new volume mount UUIDs
	newUuids := make(map[string]bool)
	volumeMountMap := make(map[string]interface{})
	for _, vm := range volumeMounts {
		vmMap, ok := vm.(map[string]interface{})
		if !ok {
			continue
		}
		uuid, _ := vmMap["uuid"].(string)
		if uuid != "" {
			newUuids[uuid] = true
			volumeMountMap[uuid] = vmMap
		}
	}
	
	// Handle removed volume mounts
	for uuid := range existingUuids {
		if !newUuids[uuid] {
			func() {
				defer func() {
					if r := recover(); r != nil {
						logging.LogError(moduleName, fmt.Sprintf("Panic in deleteVolumeMount(%s): %v", uuid, r), fmt.Errorf("%v", r))
					}
				}()
				vmm.deleteVolumeMount(uuid)
			}()
		}
	}
	
	// Handle new and updated volume mounts
	for uuid, vmValue := range volumeMountMap {
		vmMap, ok := vmValue.(map[string]interface{})
		if !ok {
			continue
		}
		
		func() {
			defer func() {
				if r := recover(); r != nil {
					logging.LogError(moduleName, fmt.Sprintf("Panic processing volume mount %s: %v", uuid, r), fmt.Errorf("%v", r))
				}
			}()
			
			if existingUuids[uuid] {
				logging.LogDebug(moduleName, fmt.Sprintf("Updating volume mount: %s", uuid))
				vmm.updateVolumeMount(vmMap)
			} else {
				logging.LogDebug(moduleName, fmt.Sprintf("Creating new volume mount: %s", uuid))
				vmm.createVolumeMount(vmMap)
			}
		}()
	}
	
	// Rebuild type cache after updates (with error handling)
	// Note: We already hold indexLock, so rebuildTypeCacheUnsafe is used
	func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogError(moduleName, fmt.Sprintf("Panic in rebuildTypeCache: %v", r), fmt.Errorf("%v", r))
			}
		}()
		vmm.rebuildTypeCacheUnsafe() // Use unsafe version since we already hold the lock
	}()
	
	// Save updated index (with error handling)
	func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogError(moduleName, fmt.Sprintf("Panic in saveIndex: %v", r), fmt.Errorf("%v", r))
			}
		}()
		vmm.saveIndex()
	}()
	
	logging.LogInfo(moduleName, "Volume mount changes processed successfully")
}

// Helper functions

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	_, err = io.Copy(destFile, sourceFile)
	return err
}

func decodeBase64(data string) (string, error) {
	// Remove quotes if present
	data = strings.Trim(data, "\"")
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// createVersionedDirectoryName creates a versioned directory name
func createVersionedDirectoryName() string {
	now := time.Now()
	timestamp := now.Format("2006_01_02_15_04_05")
	nanoseconds := now.Nanosecond() % 1_000_000_000
	return fmt.Sprintf("..%s.%09d", timestamp, nanoseconds)
}

// getTypeDirectory gets the directory name for a volume mount type
func getTypeDirectory(vmType VolumeMountType) string {
	if vmType == VolumeMountTypeSecret {
		return secretsDir
	}
	return configMapsDir
}

// parseVolumeMountType parses volume mount type from map
func parseVolumeMountType(vmMap map[string]interface{}) VolumeMountType {
	typeStr, _ := vmMap["type"].(string)
	if typeStr == "secret" {
		return VolumeMountTypeSecret
	}
	return VolumeMountTypeConfigMap
}

// setFilePermissions sets file permissions based on volume mount type
func setFilePermissions(path string, vmType VolumeMountType) error {
	if runtime.GOOS == "windows" {
		// Windows: set read-only for secrets
		if vmType == VolumeMountTypeSecret {
			return os.Chmod(path, 0400)
		}
		return os.Chmod(path, 0444)
	}
	
	// POSIX: secrets get 600, configMaps get 644
	if vmType == VolumeMountTypeSecret {
		return os.Chmod(path, 0600)
	}
	return os.Chmod(path, 0644)
}

// setSymlinkPermissions sets symlink permissions to 777 (traversal permissions)
func setSymlinkPermissions(symlinkPath string) error {
	if runtime.GOOS == "windows" {
		return nil // Windows doesn't support symlink permissions the same way
	}
	return os.Chmod(symlinkPath, 0777)
}

// createVolumeMount creates a new volume mount
func (vmm *VolumeMountManager) createVolumeMount(vmMap map[string]interface{}) {
	uuid, _ := vmMap["uuid"].(string)
	name, _ := vmMap["name"].(string)
	version, _ := vmMap["version"].(float64)
	dataObj, _ := vmMap["data"].(map[string]interface{})
	vmType := parseVolumeMountType(vmMap)
	
	logging.LogDebug(moduleName, fmt.Sprintf("Creating volume mount - UUID: %s, Name: %s, Version: %.0f, Type: %s", 
		uuid, name, version, vmType))
	
	// Create type-specific directory structure
	typeDir := getTypeDirectory(vmType)
	mountPath := filepath.Join(vmm.baseDirectory, typeDir, name)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		logging.LogError(moduleName, "Error creating mount directory", err)
		return
	}
	
	// Create versioned directory
	versionDirName := createVersionedDirectoryName()
	versionDir := filepath.Join(mountPath, versionDirName)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		logging.LogError(moduleName, "Error creating versioned directory", err)
		return
	}
	
	// Create files in versioned directory
	dataBuilder := make(map[string]interface{})
	for key, value := range dataObj {
		valueStr, ok := value.(string)
		if !ok {
			continue
		}
		
		decodedContent, err := decodeBase64(valueStr)
		if err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error decoding base64 for key: %s", key), err)
			continue
		}
		
		filePath := filepath.Join(versionDir, key)
		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error creating parent directory for: %s", key), err)
			continue
		}
		
		if err := os.WriteFile(filePath, []byte(decodedContent), 0644); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error creating file: %s", key), err)
			continue
		}
		
		// Set file permissions based on type
		if err := setFilePermissions(filePath, vmType); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error setting file permissions for: %s: %v", key, err))
		}
		
		// Store key in index
		dataBuilder[key] = key
	}
	
	// Create ..data symlink pointing to versioned directory
	dataLink := filepath.Join(mountPath, dataSymlink)
	if err := os.Remove(dataLink); err != nil && !os.IsNotExist(err) {
		logging.LogWarn(moduleName, fmt.Sprintf("Error removing existing data symlink: %v", err))
	}
	
	if err := os.Symlink(versionDirName, dataLink); err != nil {
		logging.LogError(moduleName, "Error creating data symlink", err)
		return
	}
	if err := setSymlinkPermissions(dataLink); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error setting symlink permissions: %v", err))
	}
	
	// Create per-key symlinks
	for key := range dataBuilder {
		keyLink := filepath.Join(mountPath, key)
		if err := os.Remove(keyLink); err != nil && !os.IsNotExist(err) {
			logging.LogWarn(moduleName, fmt.Sprintf("Error removing existing key symlink: %s: %v", key, err))
		}
		
		// Use relative path for symlink (works in containers)
		// Relative path: ..data/key (not filepath.Join which creates absolute paths)
		linkTarget := dataSymlink + "/" + key
		if err := os.Symlink(linkTarget, keyLink); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error creating symlink for key: %s", key), err)
			continue
		}
		if err := setSymlinkPermissions(keyLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error setting symlink permissions for key: %s: %v", key, err))
		}
	}
	
	// Update index with new schema
	mountData := map[string]interface{}{
		"name":         name,
		"type":         map[string]string{string(VolumeMountTypeSecret): "secret", string(VolumeMountTypeConfigMap): "configMap"}[string(vmType)],
		"version":      version,
		"data":         dataBuilder,
		"microservices": []interface{}{},
	}
	
	vmm.indexData[uuid] = mountData
	vmm.saveIndex()
	
	logging.LogDebug(moduleName, fmt.Sprintf("Volume mount created successfully: %s", uuid))
}

// updateVolumeMount updates an existing volume mount
func (vmm *VolumeMountManager) updateVolumeMount(vmMap map[string]interface{}) {
	uuid, _ := vmMap["uuid"].(string)
	name, _ := vmMap["name"].(string)
	version, _ := vmMap["version"].(float64)
	dataObj, _ := vmMap["data"].(map[string]interface{})
	vmType := parseVolumeMountType(vmMap)
	
	logging.LogDebug(moduleName, fmt.Sprintf("Updating volume mount - UUID: %s, Name: %s, Version: %.0f, Type: %s", 
		uuid, name, version, vmType))
	
	// Get current version from index
	currentMount, ok := vmm.indexData[uuid].(map[string]interface{})
	if ok {
		currentVersion, _ := currentMount["version"].(float64)
		if version <= currentVersion {
			logging.LogWarn(moduleName, fmt.Sprintf("Skipping update - new version %.0f not greater than current version %.0f", 
				version, currentVersion))
			return
		}
	}
	
	// Get old keys for deletion
	oldKeys := make(map[string]bool)
	if currentMount != nil {
		if oldData, ok := currentMount["data"].(map[string]interface{}); ok {
			for key := range oldData {
				oldKeys[key] = true
			}
		}
	}
	
	newKeys := make(map[string]bool)
	for key := range dataObj {
		newKeys[key] = true
	}
	
	// Find deleted keys
	deletedKeys := make(map[string]bool)
	for key := range oldKeys {
		if !newKeys[key] {
			deletedKeys[key] = true
		}
	}
	
	// Create type-specific directory structure
	typeDir := getTypeDirectory(vmType)
	mountPath := filepath.Join(vmm.baseDirectory, typeDir, name)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		logging.LogError(moduleName, "Error creating mount directory", err)
		return
	}
	
	// Create new versioned directory
	versionDirName := createVersionedDirectoryName()
	versionDir := filepath.Join(mountPath, versionDirName)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		logging.LogError(moduleName, "Error creating versioned directory", err)
		return
	}
	
	// Create files in new versioned directory
	dataBuilder := make(map[string]interface{})
	for key, value := range dataObj {
		valueStr, ok := value.(string)
		if !ok {
			continue
		}
		
		decodedContent, err := decodeBase64(valueStr)
		if err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error decoding base64 for key: %s", key), err)
			continue
		}
		
		filePath := filepath.Join(versionDir, key)
		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error creating parent directory for: %s", key), err)
			continue
		}
		
		if err := os.WriteFile(filePath, []byte(decodedContent), 0644); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error updating file: %s", key), err)
			continue
		}
		
		// Set file permissions based on type
		if err := setFilePermissions(filePath, vmType); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error setting file permissions for: %s: %v", key, err))
		}
		
		// Store key in index
		dataBuilder[key] = key
	}
	
	// Atomically swap ..data symlink to point to new version
	dataLink := filepath.Join(mountPath, dataSymlink)
	newDataLink := filepath.Join(mountPath, dataSymlink+".tmp")
	if err := os.Remove(newDataLink); err != nil && !os.IsNotExist(err) {
		logging.LogWarn(moduleName, fmt.Sprintf("Error removing temp data symlink: %v", err))
	}
	
	if err := os.Symlink(versionDirName, newDataLink); err != nil {
		logging.LogError(moduleName, "Error creating temp data symlink", err)
		return
	}
	if err := setSymlinkPermissions(newDataLink); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error setting temp symlink permissions: %v", err))
	}
	
	// Atomic move
	if err := os.Rename(newDataLink, dataLink); err != nil {
		logging.LogError(moduleName, "Error atomically moving data symlink", err)
		return
	}
	
	// Update per-key symlinks (create new, update existing)
	for key := range dataBuilder {
		keyLink := filepath.Join(mountPath, key)
		if err := os.Remove(keyLink); err != nil && !os.IsNotExist(err) {
			logging.LogWarn(moduleName, fmt.Sprintf("Error removing existing key symlink: %s: %v", key, err))
		}
		
		// Use relative path for symlink (works in containers)
		// Relative path: ..data/key (not filepath.Join which creates absolute paths)
		linkTarget := dataSymlink + "/" + key
		if err := os.Symlink(linkTarget, keyLink); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error updating symlink for key: %s", key), err)
			continue
		}
		if err := setSymlinkPermissions(keyLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error setting symlink permissions for key: %s: %v", key, err))
		}
	}
	
	// Delete symlinks for removed keys
	for key := range deletedKeys {
		keyLink := filepath.Join(mountPath, key)
		if err := os.Remove(keyLink); err != nil && !os.IsNotExist(err) {
			logging.LogWarn(moduleName, fmt.Sprintf("Error deleting symlink for removed key: %s: %v", key, err))
		} else {
			logging.LogDebug(moduleName, fmt.Sprintf("Deleted symlink for removed key: %s", key))
		}
	}
	
	// Clean up old version directories
	vmm.cleanupOldVersions(mountPath)
	
	// Get existing microservices list
	microservicesArray := []interface{}{}
	if currentMount != nil {
		if ms, ok := currentMount["microservices"].([]interface{}); ok {
			microservicesArray = ms
		}
	}
	
	// Update index with new schema
	mountData := map[string]interface{}{
		"name":         name,
		"type":         map[string]string{string(VolumeMountTypeSecret): "secret", string(VolumeMountTypeConfigMap): "configMap"}[string(vmType)],
		"version":      version,
		"data":         dataBuilder,
		"microservices": microservicesArray,
	}
	
	vmm.indexData[uuid] = mountData
	vmm.saveIndex()
	
	// Sync symlinks in per-microservice directories to reflect the update
	vmm.syncMicroserviceSymlinks(name, vmType)
	
	logging.LogDebug(moduleName, fmt.Sprintf("Volume mount updated successfully: %s", uuid))
}

// deleteVolumeMount deletes a volume mount
func (vmm *VolumeMountManager) deleteVolumeMount(uuid string) {
	logging.LogDebug(moduleName, fmt.Sprintf("Deleting volume mount: %s", uuid))
	
	// Get mount info from index
	mountData, ok := vmm.indexData[uuid].(map[string]interface{})
	if !ok {
		logging.LogWarn(moduleName, fmt.Sprintf("Volume mount not found: %s", uuid))
		return
	}
	
	// Delete mount directory and files
	name, _ := mountData["name"].(string)
	typeStr, _ := mountData["type"].(string)
	typeDir := secretsDir
	if typeStr == "configMap" {
		typeDir = configMapsDir
	}
	
	mountPath := filepath.Join(vmm.baseDirectory, typeDir, name)
	if _, err := os.Stat(mountPath); err == nil {
		if err := os.RemoveAll(mountPath); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error deleting mount directory: %s", mountPath), err)
		}
	}
	
	// Remove from index
	delete(vmm.indexData, uuid)
	
	// Remove from type cache
	vmm.typeCacheLock.Lock()
	delete(vmm.typeCache, name)
	vmm.typeCacheLock.Unlock()
	
	vmm.saveIndex()
	logging.LogDebug(moduleName, fmt.Sprintf("Volume mount deleted successfully: %s", uuid))
}

// cleanupOldVersions cleans up old version directories, keeping only the last MAX_VERSION_HISTORY versions
func (vmm *VolumeMountManager) cleanupOldVersions(mountPath string) {
	entries, err := os.ReadDir(mountPath)
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error reading mount path: %s: %v", mountPath, err))
		return
	}
	
	var versionDirs []os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		// Check if it's a versioned directory (starts with .. and matches pattern)
		if strings.HasPrefix(name, "..") && len(name) > 2 {
			// Simple pattern check: .. followed by timestamp pattern
			if entry.IsDir() {
				versionDirs = append(versionDirs, entry)
			}
		}
	}
	
	if len(versionDirs) <= maxVersionHistory {
		return
	}
	
	// Sort by modification time (oldest first)
	sort.Slice(versionDirs, func(i, j int) bool {
		infoI, errI := versionDirs[i].Info()
		infoJ, errJ := versionDirs[j].Info()
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().Before(infoJ.ModTime())
	})
	
	// Delete old versions, keeping last MAX_VERSION_HISTORY
	toDelete := len(versionDirs) - maxVersionHistory
	for i := 0; i < toDelete && i < len(versionDirs); i++ {
		versionPath := filepath.Join(mountPath, versionDirs[i].Name())
		if err := os.RemoveAll(versionPath); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error deleting old version: %s: %v", versionPath, err))
		}
	}
}

// getTypePrefix gets the type prefix for per-microservice directories
// Matching Java: getTypePrefix()
func getTypePrefix(vmType VolumeMountType) string {
	if vmType == VolumeMountTypeSecret {
		return "datasance.com~secret"
	}
	return "datasance.com~configmap"
}

// getMountPath gets the mount path for a microservice volume mount
// Matching Java: getMountPath()
func (vmm *VolumeMountManager) getMountPath(microserviceUUID, volumeName string, vmType VolumeMountType) string {
	typePrefix := getTypePrefix(vmType)
	return filepath.Join(vmm.baseDirectory, microservicesDir, microserviceUUID, "volumes", typePrefix, volumeName)
}

// hasSymlinks checks if mount path has symlinks (fast path check)
// Matching Java: hasSymlinks()
func (vmm *VolumeMountManager) hasSymlinks(mountPath string) bool {
	dataLink := filepath.Join(mountPath, dataSymlink)
	_, err := os.Lstat(dataLink)
	return err == nil
}

// PrepareMicroserviceVolumeMount prepares per-microservice volume mount directory with symlinks
// Fast path: returns immediately if directory exists
// Matching Java: prepareMicroserviceVolumeMount()
func (vmm *VolumeMountManager) PrepareMicroserviceVolumeMount(microserviceUUID, volumeName string, vmType VolumeMountType) string {
	mountPath := vmm.getMountPath(microserviceUUID, volumeName, vmType)
	
	// Fast path: if exists and has symlinks, skip (zero overhead)
	if vmm.hasSymlinks(mountPath) {
		return mountPath
	}
	
	// Slow path: create directory and copy files (only on first creation)
	// Log error but don't fail container creation (matching Java behavior)
	defer func() {
		if r := recover(); r != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error in PrepareMicroserviceVolumeMount: %v", r))
		}
	}()
	
	// Atomic directory creation (handles race conditions)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error creating microservice mount directory: %v", err))
		return mountPath // Return path anyway
	}
	
	// Calculate source path
	typeDir := getTypeDirectory(vmType)
	sourcePath := filepath.Join(vmm.baseDirectory, typeDir, volumeName)
	sourceDataLink := filepath.Join(sourcePath, dataSymlink)
	
	// Resolve the actual ..data symlink to get the real versioned directory
	// Matching Java: sourceDataLink.toRealPath()
	sourceVersionedDirPath, err := filepath.EvalSymlinks(sourceDataLink)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.LogWarn(moduleName, fmt.Sprintf("Source ..data symlink does not exist for: %s", volumeName))
		}
		return mountPath // Return path anyway
	}
	
	// Get the versioned directory name (e.g., ..2025_12_30_15_00_00.123456789)
	versionedDirName := filepath.Base(sourceVersionedDirPath)
	
	// Copy the versioned directory to per-microservice directory
	targetVersionedDir := filepath.Join(mountPath, versionedDirName)
	if _, err := os.Stat(targetVersionedDir); os.IsNotExist(err) {
		if err := os.MkdirAll(targetVersionedDir, 0755); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error creating target versioned directory: %v", err))
			return mountPath
		}
		
		// Copy all files from source versioned directory to target
		sourceEntries, err := os.ReadDir(sourceVersionedDirPath)
		if err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error reading source versioned directory: %v", err))
			return mountPath
		}
		
		for _, entry := range sourceEntries {
			if entry.IsDir() {
				continue
			}
			
			sourceFile := filepath.Join(sourceVersionedDirPath, entry.Name())
			targetFile := filepath.Join(targetVersionedDir, entry.Name())
			
			// Copy file
			srcData, err := os.ReadFile(sourceFile)
			if err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error reading source file %s: %v", entry.Name(), err))
				continue
			}
			
			if err := os.WriteFile(targetFile, srcData, 0644); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error copying file %s: %v", entry.Name(), err))
				continue
			}
			
			// Preserve file permissions
			if err := setFilePermissions(targetFile, vmType); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error setting file permissions for %s: %v", entry.Name(), err))
			}
		}
	}
	
	// Create ..data symlink pointing to versioned directory (relative path)
	dataLink := filepath.Join(mountPath, dataSymlink)
	if _, err := os.Lstat(dataLink); os.IsNotExist(err) {
		// Use relative path for ..data symlink (works in containers)
		// Relative path: just the versioned directory name
		if err := os.Symlink(versionedDirName, dataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error creating data symlink: %v", err))
		} else {
			if err := setSymlinkPermissions(dataLink); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error setting data symlink permissions: %v", err))
			}
		}
	}
	
	// Create key symlinks pointing to ..data/key (relative paths)
	targetEntries, err := os.ReadDir(targetVersionedDir)
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error reading target versioned directory: %v", err))
		return mountPath
	}
	
	for _, entry := range targetEntries {
		if entry.IsDir() {
			continue
		}
		
		key := entry.Name()
		keyLink := filepath.Join(mountPath, key)
		
		if _, err := os.Lstat(keyLink); os.IsNotExist(err) {
			// Create relative symlink: key -> ..data/key
			linkTarget := dataSymlink + "/" + key
			if err := os.Symlink(linkTarget, keyLink); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error creating key symlink for %s: %v", key, err))
				continue
			}
			if err := setSymlinkPermissions(keyLink); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error setting key symlink permissions for %s: %v", key, err))
			}
		}
	}
	
	// Track microservice usage in index
	vmm.trackMicroserviceUsage(volumeName, microserviceUUID, true)
	
	return mountPath
}

// trackMicroserviceUsage tracks microservice usage of volume mounts in index
// Matching Java: trackMicroserviceUsage()
func (vmm *VolumeMountManager) trackMicroserviceUsage(volumeName, microserviceUUID string, add bool) {
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()
	
	// Find volume mount by name
	for uuid, mountDataValue := range vmm.indexData {
		mountData, ok := mountDataValue.(map[string]interface{})
		if !ok {
			continue
		}
		
		name, _ := mountData["name"].(string)
		if name != volumeName {
			continue
		}
		
		// Get existing microservices array
		microservicesArray := []interface{}{}
		if ms, ok := mountData["microservices"].([]interface{}); ok {
			microservicesArray = ms
		}
		
		// Check if microservice is already in list
		found := false
		for _, ms := range microservicesArray {
			if msStr, ok := ms.(string); ok && msStr == microserviceUUID {
				found = true
				break
			}
		}
		
		if add && !found {
			microservicesArray = append(microservicesArray, microserviceUUID)
		} else if !add && found {
			// Remove microservice from list
			newArray := []interface{}{}
			for _, ms := range microservicesArray {
				if msStr, ok := ms.(string); ok && msStr != microserviceUUID {
					newArray = append(newArray, ms)
				}
			}
			microservicesArray = newArray
		}
		
		// Update mount data with new microservices list
		newMountData := make(map[string]interface{})
		for k, v := range mountData {
			newMountData[k] = v
		}
		newMountData["microservices"] = microservicesArray
		
		// Update index (using uuid to update the correct entry)
		vmm.indexData[uuid] = newMountData
		break
	}
}

// GetVolumeMountByName gets volume mount info by name
// Matching Java: getVolumeMountByName()
func (vmm *VolumeMountManager) GetVolumeMountByName(volumeName string) map[string]interface{} {
	vmm.indexLock.RLock()
	defer vmm.indexLock.RUnlock()
	
	for _, mountDataValue := range vmm.indexData {
		mountData, ok := mountDataValue.(map[string]interface{})
		if !ok {
			continue
		}
		
		name, _ := mountData["name"].(string)
		if name == volumeName {
			return mountData
		}
	}
	return nil
}

// syncMicroserviceSymlinks syncs per-microservice symlinks when source volume mount is updated
// Matching Java: syncMicroserviceSymlinks()
func (vmm *VolumeMountManager) syncMicroserviceSymlinks(volumeName string, vmType VolumeMountType) {
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()
	
	volumeMountData := vmm.GetVolumeMountByName(volumeName)
	if volumeMountData == nil {
		return
	}
	
	microservicesArray, ok := volumeMountData["microservices"].([]interface{})
	if !ok || len(microservicesArray) == 0 {
		return
	}
	
	// Get the source versioned directory
	typeDir := getTypeDirectory(vmType)
	sourcePath := filepath.Join(vmm.baseDirectory, typeDir, volumeName)
	sourceDataLink := filepath.Join(sourcePath, dataSymlink)
	
	// Resolve the actual ..data symlink to get the real versioned directory
	// Matching Java: sourceDataLink.toRealPath()
	sourceVersionedDirPath, err := filepath.EvalSymlinks(sourceDataLink)
	if err != nil {
		return
	}
	
	versionedDirName := filepath.Base(sourceVersionedDirPath)
	
	// Update symlinks for each microservice using this volume mount
	for _, msValue := range microservicesArray {
		microserviceUUID, ok := msValue.(string)
		if !ok {
			continue
		}
		
		mountPath := vmm.getMountPath(microserviceUUID, volumeName, vmType)
		
		if _, err := os.Stat(mountPath); os.IsNotExist(err) {
			continue
		}
		
		// Copy new versioned directory if it doesn't exist
		targetVersionedDir := filepath.Join(mountPath, versionedDirName)
		if _, err := os.Stat(targetVersionedDir); os.IsNotExist(err) {
			if err := os.MkdirAll(targetVersionedDir, 0755); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error creating target versioned directory: %v", err))
				continue
			}
			
			// Copy all files from source versioned directory to target
			sourceEntries, err := os.ReadDir(sourceVersionedDirPath)
			if err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error reading source versioned directory: %v", err))
				continue
			}
			
			for _, entry := range sourceEntries {
				if entry.IsDir() {
					continue
				}
				
				sourceFile := filepath.Join(sourceVersionedDirPath, entry.Name())
				targetFile := filepath.Join(targetVersionedDir, entry.Name())
				
				// Copy file
				srcData, err := os.ReadFile(sourceFile)
				if err != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("Error reading source file %s: %v", entry.Name(), err))
					continue
				}
				
				if err := os.WriteFile(targetFile, srcData, 0644); err != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("Error copying file %s: %v", entry.Name(), err))
					continue
				}
				
				// Preserve file permissions
				if err := setFilePermissions(targetFile, vmType); err != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("Error setting file permissions for %s: %v", entry.Name(), err))
				}
			}
		}
		
		// Atomically update ..data symlink to point to new version
		dataLink := filepath.Join(mountPath, dataSymlink)
		newDataLink := filepath.Join(mountPath, dataSymlink+".tmp")
		
		// Remove temp symlink if exists
		os.Remove(newDataLink)
		
		// Create new symlink
		if err := os.Symlink(versionedDirName, newDataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error creating temp data symlink: %v", err))
			continue
		}
		if err := setSymlinkPermissions(newDataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error setting temp symlink permissions: %v", err))
		}
		
		// Atomic move
		if err := os.Rename(newDataLink, dataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error atomically moving data symlink: %v", err))
			continue
		}
		
		// Update/create key symlinks (relative paths)
		targetEntries, err := os.ReadDir(targetVersionedDir)
		if err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error reading target versioned directory: %v", err))
			continue
		}
		
		for _, entry := range targetEntries {
			if entry.IsDir() {
				continue
			}
			
			key := entry.Name()
			keyLink := filepath.Join(mountPath, key)
			
			// Remove existing symlink if any
			os.Remove(keyLink)
			
			// Create relative symlink: key -> ..data/key
			linkTarget := dataSymlink + "/" + key
			if err := os.Symlink(linkTarget, keyLink); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error syncing key symlink %s for microservice %s: %v", key, microserviceUUID, err))
				continue
			}
			if err := setSymlinkPermissions(keyLink); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error setting key symlink permissions for %s: %v", key, err))
			}
		}
		
		// Remove symlinks for keys that no longer exist
		mountEntries, err := os.ReadDir(mountPath)
		if err == nil {
			for _, entry := range mountEntries {
				fileName := entry.Name()
				if fileName == dataSymlink || strings.HasPrefix(fileName, "..") {
					continue
				}
				
				// Check if it's a symlink
				entryPath := filepath.Join(mountPath, fileName)
				if linkInfo, err := os.Lstat(entryPath); err == nil {
					if linkInfo.Mode()&os.ModeSymlink != 0 {
						// Check if target file exists in versioned directory
						targetFile := filepath.Join(targetVersionedDir, fileName)
						if _, err := os.Stat(targetFile); os.IsNotExist(err) {
							// Remove obsolete symlink
							if err := os.Remove(entryPath); err != nil {
								logging.LogWarn(moduleName, fmt.Sprintf("Error removing obsolete symlink %s: %v", fileName, err))
							} else {
								logging.LogDebug(moduleName, fmt.Sprintf("Removed obsolete symlink: %s for microservice: %s", fileName, microserviceUUID))
							}
						}
					}
				}
			}
		}
		
		// Clean up old versioned directories in per-microservice directory
		vmm.cleanupOldVersions(mountPath)
	}
}

// CleanupMicroserviceVolumes cleans up per-microservice volume mount directories
// Matching Java: cleanupMicroserviceVolumes()
func (vmm *VolumeMountManager) CleanupMicroserviceVolumes(microserviceUUID string) {
	logging.LogDebug(moduleName, fmt.Sprintf("Cleaning up microservice volumes: %s", microserviceUUID))
	
	microservicePath := filepath.Join(vmm.baseDirectory, microservicesDir, microserviceUUID)
	if _, err := os.Stat(microservicePath); os.IsNotExist(err) {
		return
	}
	
	// Find all volume mounts used by this microservice from index
	vmm.indexLock.Lock()
	for _, mountDataValue := range vmm.indexData {
		mountData, ok := mountDataValue.(map[string]interface{})
		if !ok {
			continue
		}
		
		microservicesArray, ok := mountData["microservices"].([]interface{})
		if !ok {
			continue
		}
		
		for _, msValue := range microservicesArray {
			if msStr, ok := msValue.(string); ok && msStr == microserviceUUID {
				volumeName, _ := mountData["name"].(string)
				// Remove from tracking
				vmm.trackMicroserviceUsage(volumeName, microserviceUUID, false)
				break
			}
		}
	}
	vmm.indexLock.Unlock()
	
	// Delete per-microservice mount directory
	if err := os.RemoveAll(microservicePath); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error deleting microservice volume directory: %v", err))
	}
	
	logging.LogDebug(moduleName, fmt.Sprintf("Microservice volumes cleaned up: %s", microserviceUUID))
}

// Clear clears all volume mounts (matching Java: volumeMountManager.clear())
func (vmm *VolumeMountManager) Clear() error {
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()
	
	logging.LogDebug(moduleName, "Start clearing volume mounts")
	
	// Delete all volume mount directories (secrets, configMaps, microservices)
	if _, err := os.Stat(vmm.baseDirectory); err == nil {
		// Walk the directory tree and delete all files/directories
		err := filepath.Walk(vmm.baseDirectory, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Continue on error
			}
			
			// Don't delete the base directory itself
			if path == vmm.baseDirectory {
				return nil
			}
			
			// Delete the file or directory
			if err := os.RemoveAll(path); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error deleting: %s: %v", path, err))
			}
			return nil
		})
		if err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error walking directory: %v", err))
		}
	}
	
	// Delete index file and backups
	indexPath := filepath.Join(vmm.baseDirectory, indexFile)
	backupPath := filepath.Join(vmm.baseDirectory, indexFile+".bak")
	tempPath := filepath.Join(vmm.baseDirectory, indexFile+".tmp")
	
	os.Remove(indexPath)
	os.Remove(backupPath)
	os.Remove(tempPath)
	
	// Clear index data and cache
	vmm.indexData = make(map[string]interface{})
	vmm.typeCacheLock.Lock()
	vmm.typeCache = make(map[string]VolumeMountType)
	vmm.typeCacheLock.Unlock()
	
	// Update status reporter
	statusreporter.GetInstance().UpdateVolumeMountManagerStatus(0, time.Now().UnixMilli())
	
	logging.LogDebug(moduleName, "Finished clearing volume mounts")
	return nil
}
