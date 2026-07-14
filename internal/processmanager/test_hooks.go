package processmanager

import (
	"time"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

var execStartGateTimeoutForTest time.Duration

func execStartGateDuration() time.Duration {
	if execStartGateTimeoutForTest > 0 {
		return execStartGateTimeoutForTest
	}
	return ExecStartGateTimeout * time.Second
}

// SetExecStartGateTimeoutForTest shortens the sync start gate for unit tests.
func SetExecStartGateTimeoutForTest(d time.Duration) {
	execStartGateTimeoutForTest = d
}

// ResetExecStartGateTimeoutForTest restores the production sync start gate duration.
func ResetExecStartGateTimeoutForTest() {
	execStartGateTimeoutForTest = 0
}

// ResetExecRegistryForTest clears the exec session registry on the singleton.
func ResetExecRegistryForTest() {
	GetInstance().execRegistry = NewExecSessionRegistry()
}

// RegisterExecSessionForTest inserts a registry row for handler/integration tests.
func RegisterExecSessionForTest(rec *ExecSessionRecord) error {
	return GetInstance().ensureExecRegistry().Register(rec)
}

// NewProcessManagerForHandlerTest returns a minimal process manager wired for EdgeletAPI handler tests.
func NewProcessManagerForHandlerTest(eng engine.ContainerEngine) *ProcessManager {
	return &ProcessManager{
		engine:     eng,
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}
}

// ConfigureEngineForTest wires a test engine into the process manager singleton.
func ConfigureEngineForTest(eng engine.ContainerEngine) {
	pm := GetInstance()
	pm.engineMu.Lock()
	pm.engine = eng
	pm.containerManager = NewContainerManager(eng, nil, "docker")
	pm.engineMu.Unlock()
	if pm.logger == nil {
		pm.logger = logging.NewModuleLogger(ProcessManagerModuleName)
	}
}

// ConfigureControlPlaneRestartForTest wires engine metadata and optional recreate hook for facade restart tests.
func ConfigureControlPlaneRestartForTest(eng engine.ContainerEngine, engineName string, recreateFn func(*models.ControlPlaneDeployment, bool, int64) error) {
	pm := GetInstance()
	pm.engineMu.Lock()
	pm.engine = eng
	if engineName != "" {
		pm.engineName = engineName
	}
	pm.containerManager = NewContainerManager(eng, nil, pm.engineName)
	pm.recreateControlPlaneFn = recreateFn
	pm.engineMu.Unlock()
	if pm.logger == nil {
		pm.logger = logging.NewModuleLogger(ProcessManagerModuleName)
	}
}

// ResetProcessManagerEngineForTest clears test engine wiring on the singleton so later
// tests that expect an uninitialized process manager are not polluted.
func ResetProcessManagerEngineForTest() {
	pm := GetInstance()
	pm.engineMu.Lock()
	pm.engine = nil
	pm.engineName = ""
	pm.containerManager = nil
	pm.recreateControlPlaneFn = nil
	pm.engineMu.Unlock()
}
