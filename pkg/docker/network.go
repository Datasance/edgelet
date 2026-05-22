package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

const (
	iofogNetworkName = "iofog"
)

// resolveIofogBridgeNetworkName centralizes the scope->network mapping policy
// for docker-compatible engines (Docker + Podman wrapper).
func resolveIofogBridgeNetworkName(applicationName string, hostNetwork bool) string {
	_ = applicationName // Application scope is metadata-only in single-bridge mode.
	_ = hostNetwork
	return iofogNetworkName
}

// ensureIoFogNetworkExists ensures the fixed "iofog" bridge network exists.
// Must NOT be called while c.mu is held — use ensureNetworkLockFree instead.
func (c *Client) ensureIoFogNetworkExists() error {
	c.mu.RLock()
	cli := c.client
	baseCtx := c.ctx
	c.mu.RUnlock()

	return c.ensureNamedNetworkLockFree(cli, baseCtx, iofogNetworkName)
}

// ensureNetworkLockFree is the mutex-free implementation; used when the caller
// already holds c.mu (e.g. inside initDockerClient).
func (c *Client) ensureNetworkLockFree(cli *client.Client, baseCtx context.Context) error {
	return c.ensureNamedNetworkLockFree(cli, baseCtx, iofogNetworkName)
}

func (c *Client) ensureNamedNetworkLockFree(cli *client.Client, baseCtx context.Context, networkName string) error {
	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()

	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(filters.Arg("name", networkName)),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	if len(networks) > 0 {
		c.logger.Debugf("IoFog network \"%s\" already exists", networkName)
		return nil
	}

	c.logger.Infof("Creating IoFog network \"%s\"", networkName)
	_, err = cli.NetworkCreate(ctx, networkName, types.NetworkCreate{
		Driver: "bridge",
		Labels: map[string]string{
			"iofog": "true",
		},
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout creating network (exceeded 5s): %w", err)
		}
		return fmt.Errorf("failed to create network: %w", err)
	}

	c.logger.Infof("Successfully created IoFog network \"%s\"", networkName)
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
