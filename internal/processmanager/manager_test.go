package processmanager

import (
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/workloadmeta"
	"github.com/eclipse-iofog/agent/pkg/engine"
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

func TestStreamMicroserviceLogs_EngineNotInitialized(t *testing.T) {
	pm := &ProcessManager{}
	err := pm.StreamMicroserviceLogs("ms-1", &engine.TailConfig{Follow: true, Lines: 10}, nil)
	if err == nil {
		t.Fatalf("expected engine initialization error")
	}
	if !strings.Contains(err.Error(), "engine is not initialized") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Note: deleteRemainingMicroservices and updateRunningMicroservicesCount
// require initialized ProcessManager with Docker client and microservice manager.
// These are tested in integration tests.

func TestCountManagedRunningContainers_UsesCanonicalLabels(t *testing.T) {
	containers := []engine.Container{
		{
			Labels: map[string]string{
				workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
				workloadmeta.LabelMicroserviceUID: "ms-1",
			},
		},
		{
			Labels: map[string]string{
				"example.com/non-canonical-identity": "not-ms",
			},
		},
		{
			Labels: map[string]string{
				workloadmeta.LabelAppManagedBy: workloadmeta.ManagedByValue,
			},
		},
	}

	got := countManagedRunningContainers(containers)
	if got != 1 {
		t.Fatalf("expected 1 managed container, got %d", got)
	}
}
