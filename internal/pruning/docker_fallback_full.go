//go:build full

package pruning

import (
	"context"
	"fmt"

	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/engine"
)

func (m *Manager) pruneContainersDocker() {
	logging.LogWarn(moduleName, "cannot prune containers: container engine not configured")
}

func (m *Manager) pruneVolumesDocker() {
	logging.LogWarn(moduleName, "cannot prune volumes: container engine not configured")
}

func (m *Manager) deleteImageDocker(_ string) error {
	return fmt.Errorf("docker image removal not available in full flavor")
}

func (m *Manager) pruneAgentDocker() string {
	logging.LogWarn(moduleName, "cannot prune dangling images: container engine not configured")
	return "\nFailure - container engine not configured"
}

func (m *Manager) listImagesAndContainersDocker(_ context.Context) ([]engine.ImageInfo, []engine.Container, error) {
	return nil, nil, fmt.Errorf("docker image listing not available in full flavor")
}
