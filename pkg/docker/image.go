//revive:disable:nested-structs
package docker

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/pkg/imageref"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	dockerregistry "github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
)

// PullImage pulls an image from a registry with authentication
func (c *Client) PullImage(imageName, _ string, platform string, registry *models.Registry, progressCallback func(float32)) error {
	cli := c.GetClient()
	if cli == nil {
		return errors.New("docker client not initialized")
	}

	ctx := c.GetContext()
	c.logger.Infof("Pull image name \"%s\"", imageName)

	pullRef := strings.TrimSpace(imageName)
	if pullRef == "" {
		return errors.New("image name cannot be empty")
	}

	// Build pull options
	opts := image.PullOptions{}

	// Set platform if specified
	if platform != "" {
		opts.Platform = platform
	}

	// Set authentication if registry is not public
	if registry != nil && !registry.IsPublic && registry.URL != "from_cache" {
		serverAddress := imageref.SanitizeRegistryHost(registry.URL)
		if serverAddress == "" {
			serverAddress = registry.URL
		}
		// Build auth config
		authConfig := dockerregistry.AuthConfig{
			Username:      registry.UserName,
			Password:      registry.Password,
			ServerAddress: serverAddress,
		}
		if registry.UserEmail != "" {
			authConfig.Email = registry.UserEmail
		}

		// Encode auth config to JSON and then base64
		authJSON, err := json.Marshal(authConfig) // #nosec G117 -- Docker registry API requires password in auth JSON blob
		if err != nil {
			return fmt.Errorf("failed to marshal auth config: %w", err)
		}
		opts.RegistryAuth = base64.URLEncoding.EncodeToString(authJSON)
	}

	// Pull image
	pullResp, err := cli.ImagePull(ctx, pullRef, opts)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer func() {
		_ = pullResp.Close()
	}()

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
		return false, errors.New("docker client not initialized")
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
		return errors.New("docker client not initialized")
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

// LoadedImage is one entry produced by ImageLoad import stream.
type LoadedImage struct {
	Name string
	ID   string
}

// readPullProgress parses Docker's newline-delimited JSON pull stream and reports
// per-layer progress.
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
		return nil, errors.New("docker client not initialized")
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
		return nil, errors.New("docker client not initialized")
	}

	ctx := c.GetContext()
	images, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return nil, err
	}

	return images, nil
}

// DockerPrune prunes Docker images (removes unused images)
func (c *Client) DockerPrune() (image.PruneReport, error) {
	cli := c.GetClient()
	if cli == nil {
		return image.PruneReport{}, errors.New("docker client not initialized")
	}

	ctx := c.GetContext()
	report, err := cli.ImagesPrune(ctx, filters.NewArgs())
	if err != nil {
		return image.PruneReport{}, err
	}

	return report, nil
}

// PruneContainers removes stopped containers.
func (c *Client) PruneContainers() (container.PruneReport, error) {
	cli := c.GetClient()
	if cli == nil {
		return container.PruneReport{}, errors.New("docker client not initialized")
	}

	ctx := c.GetContext()
	report, err := cli.ContainersPrune(ctx, filters.NewArgs())
	if err != nil {
		return container.PruneReport{}, err
	}
	return report, nil
}

// PruneVolumes removes unused local volumes.
func (c *Client) PruneVolumes() (volume.PruneReport, error) {
	cli := c.GetClient()
	if cli == nil {
		return volume.PruneReport{}, errors.New("docker client not initialized")
	}

	ctx := c.GetContext()
	report, err := cli.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		return volume.PruneReport{}, err
	}
	return report, nil
}

// LoadImage imports a Docker image archive stream.
func (c *Client) LoadImage(archive io.Reader) ([]LoadedImage, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, errors.New("docker client not initialized")
	}
	ctx := c.GetContext()
	resp, err := cli.ImageLoad(ctx, archive, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	loaded := make([]LoadedImage, 0)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Docker emits messages like: "Loaded image: repo:tag"
		if strings.HasPrefix(line, "Loaded image:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "Loaded image:"))
			loaded = append(loaded, LoadedImage{Name: name})
			continue
		}
		// Podman-compatible API may emit JSON lines.
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			if v, ok := item["stream"].(string); ok && strings.Contains(v, "Loaded image:") {
				name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "Loaded image:"))
				if name != "" {
					loaded = append(loaded, LoadedImage{Name: name})
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return loaded, err
	}
	return loaded, nil
}
