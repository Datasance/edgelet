package processmanager

import (
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

// ConfigureEngineForTest wires a test engine into the process manager singleton.
func ConfigureEngineForTest(eng engine.ContainerEngine) {
	pm := GetInstance()
	pm.engine = eng
	pm.containerManager = NewContainerManager(eng, nil, "docker")
	if pm.logger == nil {
		pm.logger = logging.NewModuleLogger(ProcessManagerModuleName)
	}
}

// ConfigureControlPlaneRestartForTest wires engine metadata and optional recreate hook for facade restart tests.
func ConfigureControlPlaneRestartForTest(eng engine.ContainerEngine, engineName string, recreateFn func(*models.ControlPlaneDeployment, bool, int64) error) {
	pm := GetInstance()
	pm.engine = eng
	if engineName != "" {
		pm.engineName = engineName
	}
	pm.containerManager = NewContainerManager(eng, nil, pm.engineName)
	pm.recreateControlPlaneFn = recreateFn
	if pm.logger == nil {
		pm.logger = logging.NewModuleLogger(ProcessManagerModuleName)
	}
}
