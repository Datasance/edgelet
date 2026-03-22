package diagnostics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/pkg/docker"
)

const (
	moduleName = "Image Download Manager"
)

// FileUploader interface for uploading files to controller
// This avoids import cycle with fieldagent package
type FileUploader interface {
	UploadFile(ctx context.Context, command string, filePath string) error
}

// Manager manages Docker image snapshot creation and upload
type Manager struct {
	config       *config.Config
	fileUploader FileUploader // Optional file uploader (set by FieldAgent)
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton Image Download Manager instance
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			config: config.GetInstance(),
		}
	})
	return instance
}

// SetFileUploader sets the file uploader (called by FieldAgent during initialization)
func (m *Manager) SetFileUploader(uploader FileUploader) {
	m.fileUploader = uploader
}

// CreateImageSnapshot creates a Docker image snapshot (docker save) and compresses it
func (m *Manager) CreateImageSnapshot(ctx context.Context, imageName string) (string, error) {
	logging.LogInfo(moduleName, fmt.Sprintf("Creating image snapshot: %s", imageName))

	// Create temporary directory
	tmpDir := filepath.Join(os.TempDir(), "iofog-image-snapshots")
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Create snapshot file path
	timestamp := time.Now().Format("20060102-150405")
	snapshotFile := filepath.Join(tmpDir, fmt.Sprintf("%s-%s.tar", sanitizeImageName(imageName), timestamp))

	// Run docker save
	cmd := exec.CommandContext(ctx, "docker", "save", "-o", snapshotFile, imageName) // #nosec G204 -- binary is docker constant; args are internal snapshot path and image name
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	// Compress with gzip
	compressedFile := snapshotFile + ".gz"
	cmd = exec.CommandContext(ctx, "gzip", "-c", snapshotFile) // #nosec G204 -- binary is gzip constant; arg is an internal temp file path
	output, err := os.Create(filepath.Clean(compressedFile))
	if err != nil {
		if rerr := os.Remove(snapshotFile); rerr != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to remove snapshot file: %v", rerr))
		}
		return "", fmt.Errorf("failed to create compressed file: %w", err)
	}
	defer output.Close()

	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		if rerr := os.Remove(snapshotFile); rerr != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to remove snapshot file: %v", rerr))
		}
		if rerr := os.Remove(compressedFile); rerr != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to remove compressed file: %v", rerr))
		}
		return "", fmt.Errorf("failed to compress image: %w", err)
	}

	// Remove uncompressed file
	if rerr := os.Remove(snapshotFile); rerr != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Failed to remove snapshot file: %v", rerr))
	}

	logging.LogInfo(moduleName, fmt.Sprintf("Image snapshot created: %s", compressedFile))
	return compressedFile, nil
}

// UploadImageSnapshot uploads an image snapshot to the controller
func (m *Manager) UploadImageSnapshot(ctx context.Context, snapshotPath string) error {
	logging.LogInfo(moduleName, fmt.Sprintf("Uploading image snapshot: %s", snapshotPath))

	if m.fileUploader == nil {
		return fmt.Errorf("file uploader not set - FieldAgent not initialized")
	}

	// Upload file using the uploader
	if err := m.fileUploader.UploadFile(ctx, "image-snapshot", snapshotPath); err != nil {
		return fmt.Errorf("failed to upload image snapshot: %w", err)
	}

	logging.LogInfo(moduleName, fmt.Sprintf("Image snapshot uploaded successfully: %s", snapshotPath))
	return nil
}

// CreateImageSnapshotForMicroservice creates an image snapshot for a specific microservice
// This matches Java: createImageSnapshot() method
func (m *Manager) CreateImageSnapshotForMicroservice(ctx context.Context, microserviceUUID string) error {
	logging.LogDebug(moduleName, fmt.Sprintf("Start Create image snapshot: %s", microserviceUUID))

	// Get container for microservice
	dockerClient := docker.GetInstance()
	container, err := dockerClient.GetContainer(microserviceUUID)
	if err != nil {
		return fmt.Errorf("failed to get container: %w", err)
	}

	if container == nil {
		logging.LogWarn(moduleName, "Image snapshot: container not running")
		return fmt.Errorf("container not found for microservice: %s", microserviceUUID)
	}

	// Get image name from container
	imageName := container.Image
	if imageName == "" {
		return fmt.Errorf("container image is empty")
	}

	// Create snapshot
	snapshotPath, err := m.CreateImageSnapshot(ctx, imageName)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Upload snapshot
	if err := m.UploadImageSnapshot(ctx, snapshotPath); err != nil {
		// Cleanup on error
		if cerr := m.CleanupSnapshot(snapshotPath); cerr != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to cleanup snapshot: %v", cerr))
		}
		return err
	}

	// Cleanup after successful upload
	if err := m.CleanupSnapshot(snapshotPath); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Failed to cleanup snapshot: %v", err))
	} else {
		logging.LogInfo(moduleName, fmt.Sprintf("Image snapshot %s deleted", filepath.Base(snapshotPath)))
	}

	logging.LogDebug(moduleName, "Finished Create image snapshot")
	return nil
}

// CleanupSnapshot removes a snapshot file
func (m *Manager) CleanupSnapshot(snapshotPath string) error {
	return os.Remove(snapshotPath)
}

// sanitizeImageName sanitizes an image name for use in file paths
func sanitizeImageName(imageName string) string {
	// Replace invalid characters with underscores
	result := ""
	for _, char := range imageName {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			result += string(char)
		} else {
			result += "_"
		}
	}
	return result
}
