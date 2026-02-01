package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/eclipse-iofog/agent-go/internal/config"
)

// ensureNamespaceNetworkExists ensures the namespace network exists
func (c *Client) ensureNamespaceNetworkExists() error {
	// Get client without lock (we're already in locked context from Init)
	c.mu.RLock()
	cli := c.client
	c.mu.RUnlock()

	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	cfg := config.GetInstance()
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = "iofog"
	}

	networkName := fmt.Sprintf("iofog_%s", namespace)

	// Get context without lock (we're already in locked context from Init)
	c.mu.RLock()
	baseCtx := c.ctx
	c.mu.RUnlock()

	// Create timeout context for network operations (5 second timeout)
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()

	// Check if network exists
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(filters.Arg("name", networkName)),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	// Network exists
	if len(networks) > 0 {
		c.logger.Debugf("Namespace network \"%s\" already exists", networkName)
		return nil
	}

	// Create network (use same timeout context)
	c.logger.Infof("Creating namespace network \"%s\"", networkName)
	_, err = cli.NetworkCreate(ctx, networkName, types.NetworkCreate{
		Driver: "bridge",
		Labels: map[string]string{
			"iofog.namespace": namespace,
		},
	})
	if err != nil {
		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout creating network (exceeded 5s): %w", err)
		}
		return fmt.Errorf("failed to create network: %w", err)
	}

	c.logger.Infof("Successfully created namespace network \"%s\"", networkName)
	return nil
}

// GetDockerBridgeName returns the Docker default bridge name
func (c *Client) GetDockerBridgeName() (string, error) {
	c.mu.RLock()
	cli := c.client
	ctx := c.ctx
	c.mu.RUnlock()

	if cli == nil {
		return "", fmt.Errorf("Docker client not initialized")
	}
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return "", err
	}

	for _, network := range networks {
		if network.Options != nil {
			if val, ok := network.Options["com.docker.network.bridge.default_bridge"]; ok && val == "true" {
				if name, ok := network.Options["com.docker.network.bridge.name"]; ok {
					return name, nil
				}
			}
		}
	}

	return "", nil
}
