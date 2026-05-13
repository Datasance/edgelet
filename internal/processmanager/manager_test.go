package processmanager

import (
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/models"
)

func TestProcessManager_GetInstance(t *testing.T) {
	pm1 := GetInstance()
	pm2 := GetInstance()

	if pm1 != pm2 {
		t.Error("GetInstance should return the same instance")
	}
}

func TestProcessManager_GetName(t *testing.T) {
	pm := GetInstance()
	name := pm.GetName()

	if name != ProcessManagerModuleName {
		t.Errorf("Expected %s, got %s", ProcessManagerModuleName, name)
	}
}

func TestProcessManager_GetModuleIndex(t *testing.T) {
	pm := GetInstance()
	index := pm.GetModuleIndex()

	if index < 0 {
		t.Error("Module index should be non-negative")
	}
}

func TestLaunchLocalMicroserviceWithProgress_EngineNotInitialized(t *testing.T) {
	pm := &ProcessManager{}
	called := false
	_, err := pm.LaunchLocalMicroserviceWithProgress(&models.Microservice{}, models.NewRegistry(2, "from_cache", true, "", "", ""), "", func(stage string, message string) {
		called = true
		_ = stage
		_ = message
	})
	if err == nil {
		t.Fatalf("expected engine initialization error")
	}
	if !strings.Contains(err.Error(), "engine is not initialized") {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatalf("expected no callback events when engine is missing")
	}
}

// Note: deleteRemainingMicroservices and updateRunningMicroservicesCount
// require initialized ProcessManager with Docker client and microservice manager.
// These are tested in integration tests.
