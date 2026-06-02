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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/statusreporter"
	"github.com/datasance/edgelet/internal/store"
	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	moduleName        = "VolumeMountManager"
	volumesDir        = "volumes"
	secretsDir        = "secrets"
	configMapsDir     = "configMaps"
	microservicesDir  = "microservices"
	maxVersionHistory = 1 // Keep only current version for simplicity
	dataSymlink       = "..data"

	// Permission modes: internal storage uses 750 (owner+group only); bind-mount
	// targets use 755 dirs and 644 files so non-root container processes can access.
	internalDirMode   = 0750 // #nosec G301 -- internal volume storage (secrets/configMaps)
	bindMountDirMode  = 0755 // #nosec G301 -- bind-mount targets must be world-traversable for non-root uid
	bindMountFileMode = 0644 // #nosec G306 -- bind-mount files readable by non-root containers
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

	// Create volumes directory; 0750 restricts access to owner+group (internal storage)
	initErr := os.MkdirAll(vmm.baseDirectory, internalDirMode)
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

		// Rehydrate main directory structure from SQLite if it does not exist
		vmm.rehydrateMainStructureFromRecord(&rec)
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Loaded %d volume mounts from SQLite", len(vmm.indexData)))
}

// rehydrateMainStructureFromRecord creates the main directory structure on disk from a SQLite record
// when it does not exist (e.g. after agent restart with controller unreachable).
func (vmm *VolumeMountManager) rehydrateMainStructureFromRecord(rec *store.VolumeMountRecord) {
	if len(rec.Data) == 0 {
		return
	}

	vmType := VolumeMountTypeSecret
	if rec.Kind == "CONFIGMAP" {
		vmType = VolumeMountTypeConfigMap
	}

	typeDir := getTypeDirectory(vmType)
	mountPath := filepath.Join(vmm.baseDirectory, typeDir, rec.Name)
	sourceDataLink := filepath.Join(mountPath, dataSymlink)

	if _, err := os.Lstat(sourceDataLink); err == nil {
		// Main structure already exists
		return
	}

	if _, err := vmm.createMainStructureFromData(mountPath, rec.Data, vmType, false); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Rehydration failed for volume mount %s: %v", rec.Name, err))
		return
	}
	logging.LogDebug(moduleName, fmt.Sprintf("Rehydrated main structure for volume mount: %s", rec.Name))
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
// handles errors gracefully
func (vmm *VolumeMountManager) ProcessVolumeMountChanges(volumeMounts []interface{}) {
	// Use defer/recover to catch any panics
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

// dirMode returns the directory permission mode for internal volume storage.
// Internal dirs (secrets/, configMaps/, version dirs) use 0750 (owner+group only).
// Bind-mount targets use bindMountDirMode (0755) so non-root containers can traverse.
func dirMode(_ VolumeMountType) os.FileMode {
	return internalDirMode
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

// copyVersionedDirRecursively recursively copies from internal storage to a bind-mount target.
// dirs: 0755 (world-traversable for non-root containers); files: 0644 (caller passes bindMountFileMode).
func copyVersionedDirRecursively(src, dst string, fileMode os.FileMode) error {
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
			return os.MkdirAll(targetPath, bindMountDirMode)
		}

		data, err := os.ReadFile(path) // #nosec G304 -- path is from filepath.Walk over a known system directory
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), bindMountDirMode); err != nil {
			return err
		}

		if err := os.WriteFile(targetPath, data, fileMode); err != nil { // #nosec G306 -- bind-mount files 0644
			return err
		}

		return os.Chmod(targetPath, fileMode) // #nosec G302 -- bind-mount files 0644
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
		// Bind-mount dirs need 0755 for non-root container traversal
		symlinkDirErr := os.MkdirAll(filepath.Dir(symlinkPath), bindMountDirMode)
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

// createMainStructureFromData creates the main directory structure (versioned dir, files, ..data symlink, key symlinks)
// from the given data map (key->base64). Used by createVolumeMount, updateVolumeMount, and rehydration.
// If atomicSymlink is true, creates the ..data symlink atomically (write to .tmp then rename).
func (vmm *VolumeMountManager) createMainStructureFromData(mountPath string, dataObj map[string]interface{}, vmType VolumeMountType, atomicSymlink bool) (versionDirName string, err error) {
	if err := os.MkdirAll(mountPath, dirMode(vmType)); err != nil {
		return "", fmt.Errorf("create mount directory: %w", err)
	}

	versionDirName = createVersionedDirectoryName()
	versionDir := filepath.Join(mountPath, versionDirName)
	if err := os.MkdirAll(versionDir, dirMode(vmType)); err != nil {
		return "", fmt.Errorf("create versioned directory: %w", err)
	}

	for key, value := range dataObj {
		valueStr, ok := value.(string)
		if !ok {
			continue
		}

		decodedContent, decodeErr := decodeBase64(valueStr)
		if decodeErr != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error decoding base64 for key: %s", key), decodeErr)
			continue
		}

		filePath := filepath.Join(versionDir, key)
		if err := os.MkdirAll(filepath.Dir(filePath), dirMode(vmType)); err != nil {
			return "", fmt.Errorf("create parent for %s: %w", key, err)
		}

		if err := os.WriteFile(filePath, []byte(decodedContent), fileMode(vmType)); err != nil {
			return "", fmt.Errorf("write file %s: %w", key, err)
		}

		if err := setFilePermissions(filePath, vmType); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error setting file permissions for: %s: %v", key, err))
		}
	}

	dataLink := filepath.Join(mountPath, dataSymlink)
	symlinkTarget := dataLink
	if atomicSymlink {
		symlinkTarget = filepath.Join(mountPath, dataSymlink+".tmp")
		if err := os.Remove(symlinkTarget); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove temp data symlink: %w", err)
		}
	} else {
		if err := os.Remove(dataLink); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove existing data symlink: %w", err)
		}
	}
	if err := os.Symlink(versionDirName, symlinkTarget); err != nil {
		return "", fmt.Errorf("create data symlink: %w", err)
	}
	if err := setSymlinkPermissions(symlinkTarget); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error setting data symlink permissions: %v", err))
	}
	if atomicSymlink {
		if err := os.Rename(symlinkTarget, dataLink); err != nil {
			return "", fmt.Errorf("atomic rename data symlink: %w", err)
		}
	}

	if err := createKeySymlinksRecursively(mountPath, versionDir); err != nil {
		return "", fmt.Errorf("create key symlinks: %w", err)
	}

	return versionDirName, nil
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

	typeDir := getTypeDirectory(vmType)
	mountPath := filepath.Join(vmm.baseDirectory, typeDir, name)
	if _, err := vmm.createMainStructureFromData(mountPath, dataObj, vmType, false); err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Error creating main structure: %v", err), err)
		return
	}

	// Compute checksum from the raw data payload (base64 values from controller)
	dataChecksumJSON, _ := json.Marshal(dataObj)
	dataChecksum := vmm.checksum(string(dataChecksumJSON))

	// Update index with new schema; store raw content (key->base64) for rehydration
	mountData := map[string]interface{}{
		"name":          name,
		"type":          map[string]string{string(VolumeMountTypeSecret): "secret", string(VolumeMountTypeConfigMap): "configMap"}[string(vmType)],
		"version":       version,
		"checksum":      dataChecksum,
		"data":          dataObj,
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

	typeDir := getTypeDirectory(vmType)
	mountPath := filepath.Join(vmm.baseDirectory, typeDir, name)
	versionDirName, err := vmm.createMainStructureFromData(mountPath, dataObj, vmType, true)
	if err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Error creating main structure: %v", err), err)
		return
	}
	versionDir := filepath.Join(mountPath, versionDirName)

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

	// Update index with new schema; store raw content (key->base64) for rehydration
	mountData := map[string]interface{}{
		"name":          name,
		"type":          map[string]string{string(VolumeMountTypeSecret): "secret", string(VolumeMountTypeConfigMap): "configMap"}[string(vmType)],
		"version":       version,
		"checksum":      dataChecksum,
		"data":          dataObj,
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
func getTypePrefix(vmType VolumeMountType) string {
	if vmType == VolumeMountTypeSecret {
		return "edgelet.iofog.org~secret"
	}
	return "edgelet.iofog.org~configmap"
}

// getMountPath gets the mount path for a microservice volume mount
func (vmm *VolumeMountManager) getMountPath(microserviceUUID, volumeName string, vmType VolumeMountType) string {
	typePrefix := getTypePrefix(vmType)
	return filepath.Join(vmm.baseDirectory, microservicesDir, microserviceUUID, "volumes", typePrefix, volumeName)
}

// PrepareMicroserviceVolumeMount prepares per-microservice volume mount directory with symlinks
func (vmm *VolumeMountManager) PrepareMicroserviceVolumeMount(microserviceUUID, volumeName string, vmType VolumeMountType) string {
	mountPath := vmm.getMountPath(microserviceUUID, volumeName, vmType)

	// Slow path: create directory and copy files
	// Log error but don't fail container creation
	defer func() {
		if r := recover(); r != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error in PrepareMicroserviceVolumeMount: %v", r))
		}
	}()

	// Atomic directory creation (handles race conditions)
	// Bind-mount targets use 0755 so non-root container processes can traverse
	if err := os.MkdirAll(mountPath, bindMountDirMode); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Error creating microservice mount directory: %v", err))
		return mountPath // Return path anyway
	}

	// Calculate source path
	typeDir := getTypeDirectory(vmType)
	sourcePath := filepath.Join(vmm.baseDirectory, typeDir, volumeName)
	sourceDataLink := filepath.Join(sourcePath, dataSymlink)

	// Resolve the actual ..data symlink to get the real versioned directory
	sourceVersionedDirPath, err := filepath.EvalSymlinks(sourceDataLink)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.LogWarn(moduleName, fmt.Sprintf("Source ..data symlink does not exist for: %s", volumeName))
		}
		return mountPath // Return path anyway
	}

	// Get the versioned directory name (e.g., ..2025_12_30_15_00_00.123456789)
	versionedDirName := filepath.Base(sourceVersionedDirPath)

	// Fast path: skip if ..data already points to the correct versioned dir.
	// Still ensure both the mount directory and the versioned directory have 0755
	// permissions so that existing volumes created with the old 0700 mode are
	// migrated transparently (non-root container processes must be able to traverse).
	dataLink := filepath.Join(mountPath, dataSymlink)
	if currentTarget, err := os.Readlink(dataLink); err == nil {
		if currentTarget == versionedDirName {
			_ = os.Chmod(mountPath, bindMountDirMode)                                  // #nosec G302 -- migrate legacy 0700 dirs
			_ = os.Chmod(filepath.Join(mountPath, versionedDirName), bindMountDirMode) // #nosec G302 -- migrate legacy 0700 dirs
			return mountPath                                                           // already up to date
		}
	}

	// Copy the versioned directory to per-microservice directory (recursively)
	// Bind-mount files use 0644 so non-root containers can read both secrets and configmaps
	targetVersionedDir := filepath.Join(mountPath, versionedDirName)
	if _, err := os.Stat(targetVersionedDir); os.IsNotExist(err) {
		if err := copyVersionedDirRecursively(sourceVersionedDirPath, targetVersionedDir, bindMountFileMode); err != nil {
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

// trackMicroserviceUsage tracks microservice usage of volume mounts in index.
func (vmm *VolumeMountManager) trackMicroserviceUsage(volumeName, microserviceUUID string, add bool) {
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()
	vmm.trackMicroserviceUsageUnsafe(volumeName, microserviceUUID, add)
}

// trackMicroserviceUsageUnsafe is the lock-free variant of trackMicroserviceUsage.
// Caller must already hold indexLock (read or write).
func (vmm *VolumeMountManager) trackMicroserviceUsageUnsafe(volumeName, microserviceUUID string, add bool) {
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
		// Bind-mount files use 0644 so non-root containers can read both secrets and configmaps
		targetVersionedDir := filepath.Join(mountPath, versionedDirName)
		if _, err := os.Stat(targetVersionedDir); os.IsNotExist(err) {
			if err := copyVersionedDirRecursively(sourceVersionedDirPath, targetVersionedDir, bindMountFileMode); err != nil {
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
func (vmm *VolumeMountManager) CleanupMicroserviceVolumes(microserviceUUID string) {
	logging.LogDebug(moduleName, fmt.Sprintf("Cleaning up microservice volumes: %s", microserviceUUID))

	microservicePath := filepath.Join(vmm.baseDirectory, microservicesDir, microserviceUUID)
	if _, err := os.Stat(microservicePath); os.IsNotExist(err) {
		return
	}

	// Find all volume mounts used by this microservice from index and remove tracking.
	// trackMicroserviceUsageUnsafe is used here to avoid a deadlock: this block already
	// holds indexLock, and trackMicroserviceUsage (the locking wrapper) would deadlock.
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
				vmm.trackMicroserviceUsageUnsafe(volumeName, microserviceUUID, false)
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

// parseRunAsUser parses "uid" or "uid:gid" and returns (uid, gid).
// If gid is omitted, gid equals uid. Returns (-1, -1) on parse failure.
func parseRunAsUser(s string) (int, int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1, -1
	}
	parts := strings.SplitN(s, ":", 2)
	uid, err := strconv.ParseInt(parts[0], 10, 0)
	if err != nil || uid < 0 {
		return -1, -1
	}
	gid := uid
	if len(parts) == 2 {
		gid, err = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 0)
		if err != nil || gid < 0 {
			return -1, -1
		}
	}
	return int(uid), int(gid)
}

// ResolveHostPath resolves the absolute host-filesystem path that should be
// bind-mounted into a container for the given volume mapping entry.
//
// For VOLUME_MOUNT type (isVolumeMount == true):
//   - Parses hostDestination as "volumeName" or "volumeName/keyName"
//   - Calls PrepareMicroserviceVolumeMount to create the per-microservice
//     directory tree with versioned data and key symlinks
//   - If a key is specified, appends it to the mount path and resolves any
//     symlinks so that the container gets the real file path
//
// For legacy "$VolumeMount/<volumeName>" format:
//   - Returns <diskDirectory>/volumes/<volumeName>
//
// For any non-absolute path (VOLUME named volumes, controller omitting type, etc.):
//   - Creates a persistent data directory at <diskDirectory>/volumes/data/<uuid>/<name>
//   - This ensures the runtime always receives a writable absolute path, matching
//     the behavior of Docker named volumes but without Docker's volume subsystem.
//
// For absolute paths (BIND and explicit host paths):
//   - Returns hostDestination unchanged.
//
// This is the engine-agnostic equivalent of docker.ResolveVolumeMountPath and
// should be called by every container engine implementation before passing
// mounts to the runtime.
//
// runAsUser is optional; when non-nil and non-empty, VOLUME-type dirs under
// volumes/data/<uuid>/<name> are chown'd to uid:gid for non-root container access.
// When runAsUser is empty, VOLUME dirs get 0777 for non-root accessibility by default.
func (vmm *VolumeMountManager) ResolveHostPath(microserviceUUID, hostDestination string, isVolumeMount bool, runAsUser *string) (string, error) {
	if isVolumeMount {
		var volumeName, keyName string
		slashIdx := strings.Index(hostDestination, "/")
		if slashIdx > 0 {
			volumeName = hostDestination[:slashIdx]
			keyName = hostDestination[slashIdx+1:]
		} else {
			volumeName = hostDestination
		}

		vmType := vmm.GetVolumeMountType(volumeName)
		if vmType == "" {
			// Default to Secret (safer) when type is not in cache, matching Docker's
			// volume.go fallback behavior. A secret with 0600 files is more restrictive
			// than a configMap with 0644 files, avoiding accidental over-permissioning.
			vmType = VolumeMountTypeSecret
		}

		mountPath := vmm.PrepareMicroserviceVolumeMount(microserviceUUID, volumeName, vmType)

		if keyName != "" {
			keyPath := filepath.Join(mountPath, keyName)
			// Resolve symlink to the real versioned file so the runtime sees
			// a concrete path rather than a dangling symlink.
			if resolved, err := filepath.EvalSymlinks(keyPath); err == nil {
				return resolved, nil
			}
			return keyPath, nil
		}

		return mountPath, nil
	}

	// Any remaining non-absolute path must be turned into an absolute host path.
	// This covers:
	//   - VOLUME-type named volumes (Docker manages these natively; containerd does not)
	//   - Mappings whose "type" field was omitted by the controller (zero value "")
	//   - Relative BIND paths (unusual, but must not reach the runtime as-is)
	// A persistent directory is created so the container always gets a real,
	// writable bind-mount source instead of a path relative to the runtime's
	// internal state directory.
	if !filepath.IsAbs(hostDestination) {
		dir := filepath.Join(vmm.baseDirectory, "data", microserviceUUID, hostDestination)
		mkdirErr := os.MkdirAll(dir, bindMountDirMode)
		if mkdirErr != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("cannot create volume dir %s: %v", dir, mkdirErr))
		}
		// Set permissions for non-root container access (VOLUME type only).
		if runAsUser != nil && *runAsUser != "" {
			uid, gid := parseRunAsUser(*runAsUser)
			if uid >= 0 {
				if chownErr := os.Chown(dir, uid, gid); chownErr != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("cannot chown volume dir %s to %d:%d: %v", dir, uid, gid, chownErr))
				}
			}
		} else {
			// RunAsUser empty: make non-root accessible by default (0777).
			// #nosec G302 -- intentional for VOLUME dirs when runAsUser unset
			if chmodErr := os.Chmod(dir, 0777); chmodErr != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("cannot chmod volume dir %s: %v", dir, chmodErr))
			}
		}
		return dir, nil
	}

	return hostDestination, nil
}

// clearWalk removes all contents under baseDir (excluding the base itself).
func (vmm *VolumeMountManager) clearWalk(baseDir string) {
	_ = filepath.Walk(baseDir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // continue walking past individual entry errors
		}
		if path == baseDir {
			return nil
		}
		if err := os.RemoveAll(path); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error deleting: %s: %v", path, err))
		}
		return nil
	})
}

// Clear clears all volume mounts
func (vmm *VolumeMountManager) Clear() error {
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()

	logging.LogDebug(moduleName, "Start clearing volume mounts")

	// Delete all volume mount directories (secrets, configMaps, microservices)
	if _, err := os.Stat(vmm.baseDirectory); err == nil {
		vmm.clearWalk(vmm.baseDirectory)
		// Retry once after a short sleep to handle races with slow container unmounts
		time.Sleep(100 * time.Millisecond)
		vmm.clearWalk(vmm.baseDirectory)
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

// ClearControllerArtifacts clears only controller-origin volume mount artifacts
// while preserving local data directories under volumes/data.
func (vmm *VolumeMountManager) ClearControllerArtifacts() error {
	vmm.indexLock.Lock()
	defer vmm.indexLock.Unlock()

	logging.LogDebug(moduleName, "Start clearing controller volume-mount artifacts")

	// Remove controller mount source roots only.
	for _, dirName := range []string{secretsDir, configMapsDir} {
		dirPath := filepath.Join(vmm.baseDirectory, dirName)
		if err := os.RemoveAll(dirPath); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Error deleting controller artifact directory %s: %v", dirPath, err))
		}
	}

	// Clear persisted volume_mounts records.
	if db := store.GetInstance(); db.Conn() != nil {
		if err := db.ClearAllVolumeMounts(); err != nil {
			logging.LogError(moduleName, "Error clearing controller volume mounts from SQLite", err)
		}
	}

	// Reset in-memory index/cache to match persisted state.
	vmm.indexData = make(map[string]interface{})
	vmm.typeCacheLock.Lock()
	vmm.typeCache = make(map[string]VolumeMountType)
	vmm.typeCacheLock.Unlock()

	statusreporter.GetInstance().UpdateVolumeMountManagerStatus(0, time.Now().UnixMilli())
	logging.LogDebug(moduleName, "Finished clearing controller volume-mount artifacts")
	return nil
}
