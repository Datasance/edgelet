//go:build linux && cgo

package containerd

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/client"
	"github.com/datasance/edgelet/internal/constants"
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
		if ns == constants.EdgeletContainerdNamespace {
			logger.Debugf("Containerd namespace %q already exists.", constants.EdgeletContainerdNamespace)
			return nil
		}
	}

	if err := c.NamespaceService().Create(ctx, constants.EdgeletContainerdNamespace, nil); err != nil {
		return fmt.Errorf("create namespace %q: %w", constants.EdgeletContainerdNamespace, err)
	}

	logger.Infof("Created containerd namespace %q.", constants.EdgeletContainerdNamespace)
	return nil
}
