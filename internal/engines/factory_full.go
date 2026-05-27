//go:build full

package engines

import (
	"fmt"

	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/pkg/engine"
	edgeletengine "github.com/datasance/edgelet/pkg/engine/edgelet"
)

// NewContainerEngine constructs the embedded edgelet engine (full flavor only).
func NewContainerEngine(engineType string, cfg engine.EngineConfig) (engine.ContainerEngine, error) {
	switch engineType {
	case constants.EngineEdgelet:
		warnIfExternalRuntimePresent()
		return edgeletengine.New(cfg.LogDir), nil

	case constants.EngineDocker, constants.EnginePodman:
		return nil, fmt.Errorf("container engine %q is not available in full flavor builds (use edgelet)", engineType)

	default:
		return nil, fmt.Errorf("unknown container engine type %q: full flavor requires edgelet", engineType)
	}
}

// WrapWithLoggingIfExternal is a no-op on full builds (edgelet engine only).
func WrapWithLoggingIfExternal(eng engine.ContainerEngine, _ string) engine.ContainerEngine {
	return eng
}
