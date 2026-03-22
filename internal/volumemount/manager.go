package volumemount

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	moduleName        = "VolumeMountManager"
	volumesDir        = "volumes"
	secretsDir        = "secrets"
	configMapsDir     = "configMaps"
	microservicesDir  = "microservices"
	maxVersionHistory = 1 // Keep only current version for simplicity
	dataSymlink       = "..data"
)

// VolumeMountType represents the type of volume mount
type VolumeMountType string

const (
	VolumeMountTypeSecret       VolumeMountType = "SECRET"
	VolumeMountTypeConfigMap    VolumeMountType = "CONFIGMAP"
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
			indexData:     make(map[string]interface{}),
			typeCache:     make(map[string]VolumeMountType),
		}
		instance.init()
	})
	return instance
}

// init initializes the volume mount manager
func (vmm *VolumeMountManager) init() {
	logging.LogDebug(moduleName, "Initializing volume mount manager")

	// Create volumes directory; 0755 is intentional: base dir holds both secrets and configmaps;
	// type-specific permissions are applied to each subdirectory
	initErr := os.MkdirAll(vmm.baseDirectory, 0755) // #nosec G301
	if initErr != nil {
		logging.LogError(moduleName, "Error creating volumes directory", initErr)
		return
	}

	// Load or create index file
	vmm.loadIndex()

	// Rebuild type cache after loading index
	vmm.rebuildTypeCache()

	logging.LogInfo(moduleName, "Volume mount manager initialized successfully")
}

// loadIndex loads the volume mount index from SQLite.
// Falls back to an empty in-memory index if the store is not yet open.
func (vmm *VolumeMountManager) loadIndex() {
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()

	db := store.GetInstance()
	if db.Conn() == nil {
		logging.LogWarn(moduleName, "SQLite not open during loadIndex — starting with empty index")
		vmm.indexData = make(map[string]interface{})
		return
	}

	records, err := db.LoadAllVolumeMounts()
	if err != nil {
		logging.LogError(moduleName, "Error loading volume mounts from SQLite", err)
		vmm.indexData = make(map[string]interface{})
		return
	}

	vmm.indexData = make(map[string]interface{}, len(records))
	for _, rec := range records {
		typeStr := kindToTypeString(rec.Kind)
		entry := map[string]interface{}{
			"name":          rec.Name,
			"type":          typeStr,
			"version":       rec.Version,
			"checksum":      rec.Checksum,
			"data":          rec.Data,
			"microservices": stringsToInterfaceSlice(rec.Microservices),
		}
		vmm.indexData[rec.UUID] = entry
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Loaded %d volume mounts from SQLite", len(vmm.indexData)))
}

// saveIndex persists the entire in-memory index to SQLite atomically.
func (vmm *VolumeMountManager) saveIndex() {
	db := store.GetInstance()
	if db.Conn() == nil {
		logging.LogWarn(moduleName, "SQLite not open during saveIndex — skipping")
		return
	}

	records := make([]store.VolumeMountRecord, 0, len(vmm.indexData))
	for uuid, mountDataValue := range vmm.indexData {
		mountData, ok := mountDataValue.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := mountData["name"].(string)
		typeStr, _ := mountData["type"].(string)
		version, _ := mountData["version"].(float64)
		checksum, _ := mountData["checksum"].(string)

		var msSlice []string
		if msRaw, ok := mountData["microservices"].([]interface{}); ok {
			for _, v := range msRaw {
				if s, ok := v.(string); ok {
					msSlice = append(msSlice, s)
				}
			}
		}
		if msSlice == nil {
			msSlice = []string{}
		}

		var dataMap map[string]interface{}
		if d, ok := mountData["data"].(map[string]interface{}); ok {
			dataMap = d
		}
		if dataMap == nil {
			dataMap = map[string]interface{}{}
		}

		records = append(records, store.VolumeMountRecord{
			UUID:          uuid,
			Name:          name,
			Kind:          typeStringToKind(typeStr),
			Version:       version,
			Checksum:      checksum,
			Microservices: msSlice,
			Data:          dataMap,
		})
	}

	if err := db.ReplaceAllVolumeMounts(records); err != nil {
		logging.LogError(moduleName, "Error saving volume mounts to SQLite", err)
	}

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

// kindToTypeString converts a SQLite Kind value ("SECRET"/"CONFIGMAP") to the
// in-memory type string ("secret"/"configMap") used throughout manager.go.
func kindToTypeString(kind string) string {
	if kind == "CONFIGMAP" {
		return "configMap"
	}
	return "secret"
}

// typeStringToKind converts the in-memory type string to the SQLite Kind value.
func typeStringToKind(typeStr string) string {
	if typeStr == "configMap" {
		return "CONFIGMAP"
	}
	return "SECRET"
}

// stringsToInterfaceSlice converts []string to []interface{} for indexData compatibility.
func stringsToInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
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

// dirMode returns the directory permission mode for the given volume mount type.
// Secrets use 0700 (owner-only), configMaps use 0755 (world-traversable).
func dirMode(vmType VolumeMountType) os.FileMode {
	if vmType == VolumeMountTypeSecret {
		return 0700
	}
	return 0755 // #nosec G301 -- configmap directories are intentionally world-traversable
}

// fileMode returns the file permission mode for the given volume mount type.
// Secrets use 0600 (owner-only), configMaps use 0644 (world-readable).
func fileMode(vmType VolumeMountType) os.FileMode {
	if vmType == VolumeMountTypeSecret {
		return 0600
	}
	return 0644 // #nosec G306 -- configmap files are intentionally world-readable
}

// setFilePermissions sets file permissions based on volume mount type
func setFilePermissions(path string, vmType VolumeMountType) error {
	if runtime.GOOS == "windows" {
		// Windows: set read-only for secrets
		if vmType == VolumeMountTypeSecret {
			return os.Chmod(path, 0400)
		}
		return os.Chmod(path, 0444) // #nosec G302 -- configmap files on Windows are intentionally readable
	}

	// POSIX: secrets get 600, configMaps get 644
	return os.Chmod(path, fileMode(vmType)) // #nosec G302 -- configmap files are intentionally world-readable; secrets get 0600
}

// setSymlinkPermissions sets symlink permissions to 777 (traversal permissions)
func setSymlinkPermissions(symlinkPath string) error {
	if runtime.GOOS == "windows" {
		return nil // Windows doesn't support symlink permissions the same way
	}
	return os.Chmod(symlinkPath, 0777) // #nosec G302 -- symlinks need 777 for traversal; actual data permissions are set on target files
}

// buildRelativeDataTarget computes the relative symlink target for a nested key path.
// For a key "dir/file", the symlink at mountPath/dir/file must point to ../..data/dir/file.
// For a flat key "mykey", the symlink at mountPath/mykey points to ..data/mykey.
func buildRelativeDataTarget(keyLink string) string {
	depth := strings.Count(keyLink, "/")
	prefix := strings.Repeat("../", depth)
	return prefix + dataSymlink + "/" + keyLink
}

// copyVersionedDirRecursively recursively copies all files from src to dst,
// preserving subdirectory structure and applying fileMode to all files.
func copyVersionedDirRecursively(src, dst string, fileMode os.FileMode) error {
	// Derive directory mode from file mode: secret files (0600) → 0700 dirs, others → 0755
	dMode := os.FileMode(0755) // #nosec G301 -- configmap dirs are intentionally world-traversable
	if fileMode == 0600 {
		dMode = 0700
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, dMode)
		}

		data, err := os.ReadFile(path) // #nosec G304 -- path is from filepath.Walk over a known system directory
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), dMode); err != nil {
			return err
		}

		if err := os.WriteFile(targetPath, data, fileMode); err != nil { // #nosec G306 -- configmap files are intentionally world-readable; secrets get 0600
			return err
		}

		return os.Chmod(targetPath, fileMode) // #nosec G302 -- configmap files are intentionally world-readable; secrets get 0600
	})
}

// createKeySymlinksRecursively walks versionedDir and for each file creates a
// corresponding symlink at mountPath/<relPath> → buildRelativeDataTarget(relPath).
func createKeySymlinksRecursively(mountPath, versionedDir string) error {
	return filepath.Walk(versionedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(versionedDir, path)
		if err != nil {
			return err
		}

		// Use forward slashes for the key path (symlink target computation)
		relPathFwd := filepath.ToSlash(relPath)
		linkTarget := buildRelativeDataTarget(relPathFwd)

		symlinkPath := filepath.Join(mountPath, relPath)
		// 0755 is intentional: symlink parent dirs need world-traversal for container access
		symlinkDirErr := os.MkdirAll(filepath.Dir(symlinkPath), 0755) // #nosec G301
		if symlinkDirErr != nil {
			return symlinkDirErr
		}

		// Remove any existing symlink/file at this path
		_ = os.Remove(symlinkPath)

		if err := os.Symlink(linkTarget, symlinkPath); err != nil {
			return err
		}

		return setSymlinkPermissions(symlinkPath)
	})
}

// removeObsoleteSymlinksRecursively walks mountPath and removes symlinks whose
// target files no longer exist in targetVersionedDir. Removes empty directories bottom-up.
func removeObsoleteSymlinksRecursively(mountPath, targetVersionedDir string) error {
	var dirsToCheck []string

	err := filepath.Walk(mountPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // continue walking past individual entry errors
		}
		if path == mountPath {
			return nil
		}

		if info.IsDir() {
			dirsToCheck = append(dirsToCheck, path)
			return nil
		}

		// Skip the ..data symlink and versioned dirs
		name := filepath.Base(path)
		if strings.HasPrefix(name, "..") {
			return nil
		}

		linfo, lerr := os.Lstat(path)
		if lerr != nil {
			return nil //nolint:nilerr // continue walking past individual entry errors
		}
		if linfo.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		// Compute the corresponding file in the versioned dir
		relPath, rerr := filepath.Rel(mountPath, path)
		if rerr != nil {
			return nil //nolint:nilerr // continue walking past individual entry errors
		}

		targetFile := filepath.Join(targetVersionedDir, relPath)
		if _, serr := os.Stat(targetFile); os.IsNotExist(serr) {
			_ = os.Remove(path)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Remove empty directories bottom-up (longest path first)
	sort.Sort(sort.Reverse(sort.StringSlice(dirsToCheck)))
	for _, dir := range dirsToCheck {
		if dir == mountPath {
			continue
		}
		entries, rerr := os.ReadDir(dir)
		if rerr == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}

	return nil
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
	if err := os.MkdirAll(mountPath, dirMode(vmType)); err != nil {
		logging.LogError(moduleName, "Error creating mount directory", err)
		return
	}

	// Create versioned directory
	versionDirName := createVersionedDirectoryName()
	versionDir := filepath.Join(mountPath, versionDirName)
	if err := os.MkdirAll(versionDir, dirMode(vmType)); err != nil {
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
		if err := os.MkdirAll(filepath.Dir(filePath), dirMode(vmType)); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error creating parent directory for: %s", key), err)
			continue
		}

		if err := os.WriteFile(filePath, []byte(decodedContent), fileMode(vmType)); err != nil {
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

	// Create per-key symlinks recursively (supports nested key paths like dir/subdir/file)
	if err := createKeySymlinksRecursively(mountPath, versionDir); err != nil {
		logging.LogError(moduleName, "Error creating key symlinks", err)
	}

	// Compute checksum from the raw data payload (base64 values from controller)
	dataChecksumJSON, _ := json.Marshal(dataObj)
	dataChecksum := vmm.checksum(string(dataChecksumJSON))

	// Update index with new schema
	mountData := map[string]interface{}{
		"name":          name,
		"type":          map[string]string{string(VolumeMountTypeSecret): "secret", string(VolumeMountTypeConfigMap): "configMap"}[string(vmType)],
		"version":       version,
		"checksum":      dataChecksum,
		"data":          dataBuilder,
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
	if err := os.MkdirAll(mountPath, dirMode(vmType)); err != nil {
		logging.LogError(moduleName, "Error creating mount directory", err)
		return
	}

	// Create new versioned directory
	versionDirName := createVersionedDirectoryName()
	versionDir := filepath.Join(mountPath, versionDirName)
	if err := os.MkdirAll(versionDir, dirMode(vmType)); err != nil {
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
		if err := os.MkdirAll(filepath.Dir(filePath), dirMode(vmType)); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error creating parent directory for: %s", key), err)
			continue
		}

		if err := os.WriteFile(filePath, []byte(decodedContent), fileMode(vmType)); err != nil {
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

	// Create/update per-key symlinks recursively (supports nested paths like dir/subdir/file)
	if err := createKeySymlinksRecursively(mountPath, versionDir); err != nil {
		logging.LogError(moduleName, "Error creating key symlinks", err)
	}

	// Remove obsolete symlinks for keys no longer present
	if err := removeObsoleteSymlinksRecursively(mountPath, versionDir); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error removing obsolete symlinks: %v", err))
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

	// Compute checksum from the raw data payload (base64 values from controller)
	dataChecksumJSON, _ := json.Marshal(dataObj)
	dataChecksum := vmm.checksum(string(dataChecksumJSON))

	// Update index with new schema
	mountData := map[string]interface{}{
		"name":          name,
		"type":          map[string]string{string(VolumeMountTypeSecret): "secret", string(VolumeMountTypeConfigMap): "configMap"}[string(vmType)],
		"version":       version,
		"checksum":      dataChecksum,
		"data":          dataBuilder,
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

// PrepareMicroserviceVolumeMount prepares per-microservice volume mount directory with symlinks
// Matching Java: prepareMicroserviceVolumeMount()
func (vmm *VolumeMountManager) PrepareMicroserviceVolumeMount(microserviceUUID, volumeName string, vmType VolumeMountType) string {
	mountPath := vmm.getMountPath(microserviceUUID, volumeName, vmType)

	// Slow path: create directory and copy files
	// Log error but don't fail container creation (matching Java behavior)
	defer func() {
		if r := recover(); r != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error in PrepareMicroserviceVolumeMount: %v", r))
		}
	}()

	// Atomic directory creation (handles race conditions)
	if err := os.MkdirAll(mountPath, dirMode(vmType)); err != nil {
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

	// Fast path: skip if ..data already points to the correct versioned dir
	dataLink := filepath.Join(mountPath, dataSymlink)
	if currentTarget, err := os.Readlink(dataLink); err == nil {
		if currentTarget == versionedDirName {
			return mountPath // already up to date
		}
	}

	// Copy the versioned directory to per-microservice directory (recursively)
	targetVersionedDir := filepath.Join(mountPath, versionedDirName)
	fileMode := os.FileMode(0644)
	if vmType == VolumeMountTypeSecret {
		fileMode = 0600
	}
	if _, err := os.Stat(targetVersionedDir); os.IsNotExist(err) {
		if err := copyVersionedDirRecursively(sourceVersionedDirPath, targetVersionedDir, fileMode); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error copying versioned directory: %v", err))
			return mountPath
		}
	}

	// Create or update ..data symlink pointing to versioned directory (relative path)
	if _, err := os.Lstat(dataLink); err == nil {
		// Remove stale symlink before recreating
		if err := os.Remove(dataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error removing stale data symlink: %v", err))
		}
	}
	if _, err := os.Lstat(dataLink); os.IsNotExist(err) {
		if err := os.Symlink(versionedDirName, dataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error creating data symlink: %v", err))
		} else {
			if err := setSymlinkPermissions(dataLink); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error setting data symlink permissions: %v", err))
			}
		}
	}

	// Create key symlinks recursively (supports nested paths like dir/subdir/file)
	if err := createKeySymlinksRecursively(mountPath, targetVersionedDir); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error creating key symlinks: %v", err))
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

		// Copy new versioned directory recursively if it doesn't exist
		targetVersionedDir := filepath.Join(mountPath, versionedDirName)
		fileMode := os.FileMode(0644)
		if vmType == VolumeMountTypeSecret {
			fileMode = 0600
		}
		if _, err := os.Stat(targetVersionedDir); os.IsNotExist(err) {
			if err := copyVersionedDirRecursively(sourceVersionedDirPath, targetVersionedDir, fileMode); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error copying versioned directory for microservice %s: %v", microserviceUUID, err))
				continue
			}
		}

		// Atomically update ..data symlink to point to new version
		dataLink := filepath.Join(mountPath, dataSymlink)
		newDataLink := filepath.Join(mountPath, dataSymlink+".tmp")

		if rerr := os.Remove(newDataLink); rerr != nil && !os.IsNotExist(rerr) {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to remove temp data symlink: %v", rerr))
		}

		if err := os.Symlink(versionedDirName, newDataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error creating temp data symlink: %v", err))
			continue
		}
		if err := setSymlinkPermissions(newDataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error setting temp symlink permissions: %v", err))
		}

		if err := os.Rename(newDataLink, dataLink); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error atomically moving data symlink: %v", err))
			continue
		}

		// Update/create key symlinks recursively (supports nested paths)
		if err := createKeySymlinksRecursively(mountPath, targetVersionedDir); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error syncing key symlinks for microservice %s: %v", microserviceUUID, err))
		}

		// Remove symlinks for keys that no longer exist
		if err := removeObsoleteSymlinksRecursively(mountPath, targetVersionedDir); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error removing obsolete symlinks for microservice %s: %v", microserviceUUID, err))
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
		err := filepath.Walk(vmm.baseDirectory, func(path string, _ os.FileInfo, err error) error {
			if err != nil {
				return nil //nolint:nilerr // continue walking past individual entry errors
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

	// Clear SQLite volume mount records
	if db := store.GetInstance(); db.Conn() != nil {
		if err := db.ClearAllVolumeMounts(); err != nil {
			logging.LogError(moduleName, "Error clearing volume mounts from SQLite", err)
		}
	}

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
