package processmanager

import (
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
