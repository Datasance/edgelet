package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	dockerregistry "github.com/docker/docker/api/types/registry"
	"github.com/eclipse-iofog/agent/internal/models"
)

// PullImage pulls an image from a registry with authentication
func (c *Client) PullImage(imageName, _ string, platform string, registry *models.Registry, progressCallback func(float32)) error {
	cli := c.GetClient()
	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	c.logger.Infof("Pull image name \"%s\"", imageName)

	// Parse image name and tag
	imgRef := imageName
	tag := "latest"

	if parts := strings.Split(imageName, ":"); len(parts) > 1 {
		imgRef = parts[0]
		tag = parts[1]
	}

	// Build pull options
	opts := image.PullOptions{}

	// Set platform if specified
	if platform != "" {
		opts.Platform = platform
	}

	// Set authentication if registry is not public
	if !registry.IsPublic && registry.URL != "from_cache" {
		// Build auth config
		authConfig := dockerregistry.AuthConfig{
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
	pullResp, err := cli.ImagePull(ctx, fmt.Sprintf("%s:%s", imgRef, tag), opts)
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
	images, err := cli.ImageList(ctx, image.ListOptions{})
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
	_, err := cli.ImageRemove(ctx, imageID, image.RemoveOptions{
		Force: true,
	})
	return err
}

// pullEvent matches Docker's ImagePull JSON stream format (one JSON object per line).
type pullEvent struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	ProgressDetail struct {
		Current int    `json:"current"`
		Total   int    `json:"total"`
		Error   string `json:"error,omitempty"`
	} `json:"progressDetail"`
}

// readPullProgress parses Docker's newline-delimited JSON pull stream and reports
// per-layer progress. Uses the same formula as Java: sum(layer_pcts)/(count-1).
func readPullProgress(reader io.Reader, callback func(float32)) error {
	dec := json.NewDecoder(reader)
	layerPct := make(map[string]int)
	for {
		var ev pullEvent
		if err := dec.Decode(&ev); err != nil {
			if err == io.EOF {
				callback(100.0)
				break
			}
			return err
		}
		if ev.ID == "" {
			continue
		}
		if ev.ProgressDetail.Total > 0 {
			pct := ev.ProgressDetail.Current * 100 / ev.ProgressDetail.Total
			if pct > 100 {
				pct = 100
			}
			if prev, ok := layerPct[ev.ID]; ok && pct < prev {
				pct = prev
			}
			layerPct[ev.ID] = pct
		} else if ev.Status == "Pull complete" || ev.Status == "Already exists" || ev.Status == "Download complete" {
			layerPct[ev.ID] = 100
		}
		if len(layerPct) == 0 {
			continue
		}
		sum := 0
		for _, p := range layerPct {
			sum += p
		}
		div := len(layerPct)
		if div > 1 {
			div--
		}
		avg := float32(sum) / float32(div)
		if avg > 100 {
			avg = 100
		}
		callback(avg)
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
func (c *Client) GetImages() ([]image.Summary, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	images, err := cli.ImageList(ctx, image.ListOptions{})
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
