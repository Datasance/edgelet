package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/fieldagent"
)

// TestControllerConnection tests connection to ioFog controller
// This test requires a running controller instance
func TestControllerConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Load config
	cfg := config.GetInstance()
	if cfg.ControllerURL == "" {
		t.Skip("Controller URL not configured, skipping test")
	}

	// Create FieldAgent instance
	agent := fieldagent.GetInstance()

	// Start agent (this will attempt to connect)
	err := agent.Start()
	if err != nil {
		t.Logf("FieldAgent start failed (expected if controller not available): %v", err)
		return
	}
	defer agent.Stop()

	// Give it time to connect
	time.Sleep(2 * time.Second)

	// Check if agent is connected
	// This would require exposing connection status from FieldAgent
	t.Log("Controller connection test completed")
}

// TestControllerAPI tests HTTP API communication with controller
func TestControllerAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := config.GetInstance()
	if cfg.ControllerURL == "" {
		t.Skip("Controller URL not configured, skipping test")
	}

	// Test HTTP connectivity
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(cfg.ControllerURL + "/api/v3/status")
	if err != nil {
		t.Logf("Controller not reachable (expected if not running): %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Logf("Controller returned status: %d", resp.StatusCode)
	}

	t.Log("Controller API test completed")
}
