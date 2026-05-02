//go:build linux

package iofogcontainerd

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/client"
	"github.com/eclipse-iofog/agent/internal/constants"
)

// ensureNamespace creates the iofog namespace in containerd if it does not
// already exist. All iofog-managed containers live in this namespace, ensuring
// they are isolated from any system containerd workloads.
func ensureNamespace(ctx context.Context, c *client.Client) error {
	namespaces, err := c.NamespaceService().List(ctx)
	if err != nil {
		return fmt.Errorf("list containerd namespaces: %w", err)
	}

	for _, ns := range namespaces {
		if ns == constants.IofogContainerdNamespace {
			logger.Debugf("Containerd namespace %q already exists.", constants.IofogContainerdNamespace)
			return nil
		}
	}

	if err := c.NamespaceService().Create(ctx, constants.IofogContainerdNamespace, nil); err != nil {
		return fmt.Errorf("create namespace %q: %w", constants.IofogContainerdNamespace, err)
	}

	logger.Infof("Created containerd namespace %q.", constants.IofogContainerdNamespace)
	return nil
}
