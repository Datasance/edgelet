package docker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/moby/moby/client"
)

const (
	edgeletNetworkName = "edgelet"
)

// resolveIofogBridgeNetworkName centralizes the scope->network mapping policy
// for docker-compatible engines (Docker + Podman wrapper).
func resolveIofogBridgeNetworkName(applicationName string, hostNetwork bool) string {
	_ = applicationName // Application scope is metadata-only in single-bridge mode.
	_ = hostNetwork
	return edgeletNetworkName
}

// ensureEdgeletNetworkExists ensures the fixed "edgelet" bridge network exists.
// Must NOT be called while c.mu is held — use ensureNetworkLockFree instead.
func (c *Client) ensureEdgeletNetworkExists() error {
	c.mu.RLock()
	cli := c.client
	baseCtx := c.ctx
	c.mu.RUnlock()

	return c.ensureNamedNetworkLockFree(baseCtx, cli, edgeletNetworkName)
}

// ensureNetworkLockFree is the mutex-free implementation; used when the caller
// already holds c.mu (e.g. inside initDockerClient).
func (c *Client) ensureNetworkLockFree(baseCtx context.Context, cli *client.Client) error {
	return c.ensureNamedNetworkLockFree(baseCtx, cli, edgeletNetworkName)
}

func (c *Client) ensureNamedNetworkLockFree(baseCtx context.Context, cli *client.Client, networkName string) error {
	if cli == nil {
		return errors.New("docker client not initialized")
	}

	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()

	listResult, err := cli.NetworkList(ctx, client.NetworkListOptions{
		Filters: make(client.Filters).Add("name", networkName),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	if len(listResult.Items) > 0 {
		c.logger.Debugf("Edgelet network \"%s\" already exists", networkName)
		return nil
	}

	c.logger.Infof("Creating Edgelet network \"%s\"", networkName)
	_, err = cli.NetworkCreate(ctx, networkName, client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"edgelet": "true",
		},
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout creating network (exceeded 5s): %w", err)
		}
		return fmt.Errorf("failed to create network: %w", err)
	}

	c.logger.Infof("Successfully created Edgelet network \"%s\"", networkName)
	return nil
}

// GetDockerBridgeName returns the Docker default bridge name
func (c *Client) GetDockerBridgeName() (string, error) {
	c.mu.RLock()
	cli := c.client
	ctx := c.ctx
	c.mu.RUnlock()

	if cli == nil {
		return "", errors.New("docker client not initialized")
	}
	listResult, err := cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return "", err
	}

	for _, net := range listResult.Items {
		if net.Options != nil {
			if val, ok := net.Options["com.docker.network.bridge.default_bridge"]; ok && val == "true" {
				if name, ok := net.Options["com.docker.network.bridge.name"]; ok {
					return name, nil
				}
			}
		}
	}

	return "", nil
}
