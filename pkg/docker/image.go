package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/eclipse-iofog/agent-go/internal/models"
)

// PullImage pulls an image from a registry with authentication
func (c *Client) PullImage(imageName, microserviceUUID, platform string, registry *models.Registry, progressCallback func(float32)) error {
	cli := c.GetClient()
	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	c.logger.Infof("Pull image name \"%s\"", imageName)

	// Parse image name and tag
	image := imageName
	tag := "latest"

	if parts := strings.Split(imageName, ":"); len(parts) > 1 {
		image = parts[0]
		tag = parts[1]
	}

	// Build pull options
	opts := types.ImagePullOptions{}

	// Set platform if specified
	if platform != "" {
		opts.Platform = platform
	}

	// Set authentication if registry is not public
	if !registry.IsPublic && registry.URL != "from_cache" {
		// Build auth config
		authConfig := types.AuthConfig{
			Username:      registry.UserName,
			Password:      registry.Password,
			ServerAddress: registry.URL,
		}
		if registry.UserEmail != "" {
			authConfig.Email = registry.UserEmail
		}

		// Encode auth config to JSON and then base64
		authJSON, err := json.Marshal(authConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal auth config: %w", err)
		}
		opts.RegistryAuth = base64.URLEncoding.EncodeToString(authJSON)
	}

	// Pull image
	pullResp, err := cli.ImagePull(ctx, fmt.Sprintf("%s:%s", image, tag), opts)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer pullResp.Close()

	// Read pull progress
	if progressCallback != nil {
		if err := readPullProgress(pullResp, progressCallback); err != nil {
			c.logger.Warnf("Error reading pull progress: %v", err)
		}
	} else {
		// Just drain the response
		_, _ = io.Copy(io.Discard, pullResp)
	}

	c.logger.Infof("Successfully pulled image \"%s\"", imageName)
	return nil
}

// FindLocalImage checks if an image exists locally
func (c *Client) FindLocalImage(imageName string) (bool, error) {
	cli := c.GetClient()
	if cli == nil {
		return false, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	images, err := cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return false, err
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageName || strings.HasPrefix(tag, imageName+":") {
				return true, nil
			}
		}
	}

	return false, nil
}

// RemoveImage removes an image by ID
func (c *Client) RemoveImage(imageID string) error {
	cli := c.GetClient()
	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	_, err := cli.ImageRemove(ctx, imageID, types.ImageRemoveOptions{
		Force: true,
	})
	return err
}

// readPullProgress reads pull progress from the response stream
func readPullProgress(reader io.Reader, callback func(float32)) error {
	// This is a simplified version - in production, you'd parse the JSON stream
	// from Docker and extract progress information
	// For now, we'll just drain the stream
	buf := make([]byte, 1024)
	for {
		_, err := reader.Read(buf)
		if err == io.EOF {
			callback(100.0)
			break
		}
		if err != nil {
			return err
		}
		// In a real implementation, parse JSON and extract progress
		// For now, we'll just call the callback periodically
	}
	return nil
}

// GetImageInspect inspects an image
func (c *Client) GetImageInspect(imageName string) (*types.ImageInspect, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	inspect, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return nil, err
	}

	return &inspect, nil
}

// GetImages returns all Docker images
func (c *Client) GetImages() ([]types.ImageSummary, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	images, err := cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	return images, nil
}

// DockerPrune prunes Docker images (removes unused images)
// This matches Java: dockerPrune()
func (c *Client) DockerPrune() (types.ImagesPruneReport, error) {
	cli := c.GetClient()
	if cli == nil {
		return types.ImagesPruneReport{}, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	report, err := cli.ImagesPrune(ctx, filters.NewArgs())
	if err != nil {
		return types.ImagesPruneReport{}, err
	}

	return report, nil
}
