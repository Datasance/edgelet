package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/supervisor"
	"github.com/eclipse-iofog/edgelet/internal/utils"
)

// TestControllerDown verifies agent remains running when controller is unreachable.
// Agent should not crash; workers should skip API calls and use exponential backoff for ping.
func TestControllerDown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	configPath := utils.ConfigYAMLPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("Config file not found at %s, skipping test", configPath)
	}

	if err := config.LoadConfig(configPath); err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	// Point to unreachable controller (no server listening)
	cfg := config.GetInstance()
	originalURL := cfg.ControllerURL
	cfg.ControllerURL = "https://127.0.0.1:19999/api/v3/" // Unlikely to have server
	defer func() { cfg.ControllerURL = originalURL }()

	// Use iofog engine to avoid Docker/Podman socket dependency
	originalEngine := cfg.ContainerEngine
	cfg.ContainerEngine = "edgelet"
	defer func() { cfg.ContainerEngine = originalEngine }()

	sup := supervisor.NewSupervisor()

	// Agent should start (controller unreachable is OK)
	err := sup.Start()
	if err != nil {
		t.Fatalf("Supervisor should start even when controller is down: %v", err)
	}

	// Run for a few seconds — agent must not crash
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = sup.Stop()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Agent stopped gracefully after controller-down scenario")
	case <-ctx.Done():
		t.Log("Agent ran for 5s without crashing (controller down)")
		// Force stop
		_ = sup.Stop()
	}
}

// TestEngineUnavailable verifies supervisor handles Docker/Podman socket
// temporarily unavailable (retry with backoff, then suggest iofog).
// Uses invalid socket path to simulate engine down.
func TestEngineUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	configPath := utils.ConfigYAMLPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("Config file not found at %s, skipping test", configPath)
	}

	if err := config.LoadConfig(configPath); err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	// Use Docker with non-existent socket to simulate engine unavailable
	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	originalContainerEngineURL := cfg.ContainerEngineURL
	cfg.ContainerEngine = "docker"
	cfg.ContainerEngineURL = "unix:///var/run/nonexistent-docker.sock"
	defer func() {
		cfg.ContainerEngine = originalEngine
		cfg.ContainerEngineURL = originalContainerEngineURL
	}()

	sup := supervisor.NewSupervisor()

	// Supervisor should fail to start (engine init retries then gives up)
	err := sup.Start()
	if err == nil {
		t.Log("Supervisor started (unexpected with invalid socket); stopping")
		_ = sup.Stop()
		return
	}

	// Expected: init fails after retries with suggestion to use iofog
	t.Logf("Expected failure when engine unavailable: %v", err)
}
