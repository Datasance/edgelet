//go:build linux

package engines

import (
	"fmt"

	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
	dockerengine "github.com/eclipse-iofog/edgelet/pkg/engine/docker"
	edgeletengine "github.com/eclipse-iofog/edgelet/pkg/engine/edgelet"
	podmanengine "github.com/eclipse-iofog/edgelet/pkg/engine/podman"
)

// NewContainerEngine constructs and returns the engine specified by engineType.
func NewContainerEngine(engineType string, cfg engine.EngineConfig) (engine.ContainerEngine, error) {
	switch engineType {
	case constants.EngineDocker:
		return dockerengine.New(), nil

	case constants.EnginePodman:
		return podmanengine.New(), nil

	case constants.EngineEdgelet:
		warnIfExternalRuntimePresent()
		return edgeletengine.New(cfg.LogDir), nil

	default:
		return nil, fmt.Errorf("unknown container engine type %q: must be one of edgelet, docker, podman", engineType)
	}
}

// WrapWithLoggingIfExternal wraps docker/podman engines with structured Debug API logging.
func WrapWithLoggingIfExternal(eng engine.ContainerEngine, engineType string) engine.ContainerEngine {
	switch engineType {
	case constants.EngineDocker, constants.EnginePodman:
		return engine.NewLoggingEngine(eng, engineType)
	default:
		return eng
	}
}
