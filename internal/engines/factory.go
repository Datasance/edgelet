package engines

import (
	"fmt"
	"os"

	"github.com/eclipse-iofog/agent/internal/constants"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/pkg/engine"
	dockerengine "github.com/eclipse-iofog/agent/pkg/engine/docker"
	iofogengine "github.com/eclipse-iofog/agent/pkg/engine/iofog"
	podmanengine "github.com/eclipse-iofog/agent/pkg/engine/podman"
)

var factoryLogger = logging.NewModuleLogger("ContainerEngine")

// NewContainerEngine constructs and returns the engine specified by engineType.
// Valid values: "docker", "podman", "iofog".
func NewContainerEngine(engineType string, cfg engine.EngineConfig) (engine.ContainerEngine, error) {
	switch engineType {
	case constants.EngineDocker:
		return dockerengine.New(), nil

	case constants.EnginePodman:
		return podmanengine.New(), nil

	case constants.EngineIofog:
		warnIfExternalRuntimePresent()
		return iofogengine.New(cfg.LogDir), nil

	default:
		return nil, fmt.Errorf("unknown container engine type %q: must be one of docker, podman, iofog", engineType)
	}
}

// WrapWithLoggingIfExternal wraps docker/podman engines with structured Debug API logging.
// Call after Init() so initialization is not double-wrapped.
func WrapWithLoggingIfExternal(eng engine.ContainerEngine, engineType string) engine.ContainerEngine {
	switch engineType {
	case constants.EngineDocker, constants.EnginePodman:
		return engine.NewLoggingEngine(eng, engineType)
	default:
		return eng
	}
}

// warnIfExternalRuntimePresent logs a warning when Docker or Podman sockets are
// found on the host while the iofog embedded engine is selected.
// This is informational only — iofog uses fully private paths and coexists
// safely with any system runtime.
func warnIfExternalRuntimePresent() {
	sockets := []struct {
		name string
		path string
	}{
		{"Docker", "/var/run/docker.sock"},
		{"Podman", "/run/podman/podman.sock"},
		{"Podman (user)", "/run/user/0/podman/podman.sock"},
	}

	for _, s := range sockets {
		if _, err := os.Stat(s.path); err == nil {
			factoryLogger.Warnf(
				"%s socket detected at %s while containerEngine=iofog is selected. "+
					"The iofog engine uses isolated private paths (/var/lib/iofog-agent-containerd/, "+
					"/run/iofog-agent/containerd.sock) and will not interfere with %s.",
				s.name, s.path, s.name,
			)
		}
	}
}
