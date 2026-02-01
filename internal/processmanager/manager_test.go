package processmanager

import (
	"testing"
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

// Note: deleteRemainingMicroservices and updateRunningMicroservicesCount
// require initialized ProcessManager with Docker client and microservice manager.
// These are tested in integration tests.
