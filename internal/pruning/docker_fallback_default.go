//go:build !lite && !full

package pruning

import (
	"context"
	"fmt"

	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/engine"
)

func (m *Manager) pruneContainersDocker() {
	logging.LogWarn(moduleName, "cannot prune containers: build with -tags lite or full")
}

func (m *Manager) pruneVolumesDocker() {
	logging.LogWarn(moduleName, "cannot prune volumes: build with -tags lite or full")
}

func (m *Manager) deleteImageDocker(_ string) error {
	return fmt.Errorf("docker image removal requires -tags lite or full")
}

func (m *Manager) pruneAgentDocker() string {
	return "\nFailure - build with -tags lite or full"
}

func (m *Manager) listImagesAndContainersDocker(_ context.Context) ([]engine.ImageInfo, []engine.Container, error) {
	return nil, nil, fmt.Errorf("docker listing requires -tags lite or full")
}
