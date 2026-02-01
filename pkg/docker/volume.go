package docker

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/volumemount"
)

// ResolveVolumeMountPath resolves volume mount paths for VOLUME_MOUNT type
// This matches Java logic: resolveVolumeMountPath()
func ResolveVolumeMountPath(hostDestination string, volumeMappingType models.VolumeMappingType, microserviceUUID string) (string, error) {
	// Handle VOLUME_MOUNT type
	if volumeMappingType == models.VolumeMappingTypeVolumeMount {
		// Parse hostDestination to extract volume name and optional key
		// Format: "volume-name" or "volume-name/key-name"
		var volumeName, keyName string
		slashIndex := strings.Index(hostDestination, "/")
		if slashIndex > 0 {
			// Key is specified: "volume-name/key-name"
			volumeName = hostDestination[:slashIndex]
			keyName = hostDestination[slashIndex+1:]
		} else {
			// No key specified: "volume-name" (mount entire directory)
			volumeName = hostDestination
		}

		// Get volume mount type from VolumeMountManager
		volumeMountManager := volumemount.GetInstance()
		volumeMountType := volumeMountManager.GetVolumeMountType(volumeName)
		if volumeMountType == "" {
			// Default to SECRET if not found
			volumeMountType = volumemount.VolumeMountTypeSecret
		}

		// Prepare per-microservice mount point (creates directory structure and symlinks)
		// Matching Java: prepareMicroserviceVolumeMount()
		mountPath := volumeMountManager.PrepareMicroserviceVolumeMount(microserviceUUID, volumeName, volumeMountType)

		// If key is specified, append it to mount path to point to specific file
		// Then resolve the symlink to get the actual file path (Docker bind mounts need real paths)
		if keyName != "" {
			keyPath := filepath.Join(mountPath, keyName)
			// Resolve symlink to get actual file path (matching Java behavior)
			// When mounting a specific file, Docker needs the resolved path, not the symlink
			// This is critical for container bind mounts to work correctly
			resolvedPath, err := filepath.EvalSymlinks(keyPath)
			if err != nil {
				// If symlink resolution fails, use the original path (may be a regular file)
				// This can happen if the file doesn't exist yet or is not a symlink
				resolvedPath = keyPath
			}
			mountPath = resolvedPath
		}

		// Check if agent is running in container
		iofogDaemon := os.Getenv("IOFOG_DAEMON")
		isContainer := strings.ToLower(iofogDaemon) == "container"

		if isContainer {
			// Agent running in container - need to check volume mounting
			dockerClient := GetInstance()
			cli := dockerClient.GetClient()
			if cli != nil {
				ctx := dockerClient.GetContext()
				// Check if iofog-agent-directory volume exists
				volumes, err := cli.VolumeList(ctx, volume.ListOptions{
					Filters: filters.NewArgs(),
				})
				if err == nil {
					for _, vol := range volumes.Volumes {
						if vol.Name == "iofog-agent-directory" {
							// Volume exists - inspect it to get mount point
							volumeInfo, err := cli.VolumeInspect(ctx, "iofog-agent-directory")
							if err == nil {
								mountPoint := volumeInfo.Mountpoint
								cfg := config.GetInstance()
								diskDir := cfg.DiskDirectory
								// Convert absolute path to relative path within volume
								if strings.HasPrefix(mountPath, diskDir) {
									return mountPoint + mountPath[len(diskDir):], nil
								}
							}
						}
					}
				}
			}
		}

		return mountPath, nil
	}

	// Legacy handling for $VolumeMount/ prefix (backward compatibility)
	if strings.HasPrefix(hostDestination, "$VolumeMount/") {
		volumeName := hostDestination[len("$VolumeMount/"):]

		// Check if agent is running in container
		iofogDaemon := os.Getenv("IOFOG_DAEMON")
		isContainer := strings.ToLower(iofogDaemon) == "container"

		cfg := config.GetInstance()
		diskDir := cfg.DiskDirectory

		if !isContainer {
			// Agent running on host - use disk directory directly
			return diskDir + "volumes/" + volumeName, nil
		}

		// Agent running in container - need to check volume mounting
		dockerClient := GetInstance()
		cli := dockerClient.GetClient()
		if cli != nil {
			ctx := dockerClient.GetContext()
			// Check if iofog-agent-directory volume exists
			volumes, err := cli.VolumeList(ctx, volume.ListOptions{
				Filters: filters.NewArgs(),
			})
			if err == nil {
				for _, vol := range volumes.Volumes {
					if vol.Name == "iofog-agent-directory" {
						// Volume exists - inspect it to get mount point
						volumeInfo, err := cli.VolumeInspect(ctx, "iofog-agent-directory")
						if err == nil {
							mountPoint := volumeInfo.Mountpoint
							return mountPoint + "/volumes/" + volumeName, nil
						}
					}
				}
			}
		}

		// Volume doesn't exist - assume bind mount, use disk directory
		return diskDir + "volumes/" + volumeName, nil
	}

	// Return as-is for BIND and VOLUME types
	return hostDestination, nil
}
