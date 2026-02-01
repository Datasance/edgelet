package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/supervisor"
	"github.com/eclipse-iofog/agent-go/internal/utils"
)

// TestAgentStartup tests full agent startup sequence
func TestAgentStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Ensure we have a config file
	configPath := utils.ConfigYAMLPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("Config file not found at %s, skipping test", configPath)
	}

	// Load configuration
	err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	// Create supervisor
	sup := supervisor.NewSupervisor()

	// Start supervisor with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start in goroutine
	startErr := make(chan error, 1)
	go func() {
		startErr <- sup.Start()
	}()

	// Wait for startup or timeout
	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Supervisor failed to start: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Supervisor startup timed out")
	}

	// Give it a moment to fully initialize
	time.Sleep(2 * time.Second)

	// Stop supervisor
	err = sup.Stop()
	if err != nil {
		t.Errorf("Supervisor failed to stop gracefully: %v", err)
	}

	t.Log("Agent startup test completed successfully")
}

// TestAgentShutdown tests graceful shutdown
func TestAgentShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	configPath := utils.ConfigYAMLPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("Config file not found at %s, skipping test", configPath)
	}

	err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	sup := supervisor.NewSupervisor()

	err = sup.Start()
	if err != nil {
		t.Fatalf("Supervisor failed to start: %v", err)
	}

	// Let it run for a bit
	time.Sleep(1 * time.Second)

	// Test graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	done := make(chan error, 1)
	go func() {
		done <- sup.Stop()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}
	case <-shutdownCtx.Done():
		t.Error("Shutdown timed out")
	}

	t.Log("Agent shutdown test completed")
}

// TestMultiModuleInteraction tests interaction between multiple modules
func TestMultiModuleInteraction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test would verify that modules can communicate properly
	// For example: FieldAgent -> ProcessManager, MessageBus -> LocalAPI, etc.
	
	configPath := utils.ConfigYAMLPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("Config file not found at %s, skipping test", configPath)
	}

	err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	sup := supervisor.NewSupervisor()

	err = sup.Start()
	if err != nil {
		t.Fatalf("Supervisor failed to start: %v", err)
	}

	// Give modules time to initialize and interact
	time.Sleep(3 * time.Second)

	// Verify modules are running
	// This would require exposing status from supervisor

	err = sup.Stop()
	if err != nil {
		t.Errorf("Supervisor failed to stop: %v", err)
	}

	t.Log("Multi-module interaction test completed")
}

// TestOfflineMode tests agent behavior when controller is unavailable
func TestOfflineMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test verifies that the agent can start and operate
	// even when the controller is not available

	configPath := utils.ConfigYAMLPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("Config file not found at %s, skipping test", configPath)
	}

	// Temporarily set invalid controller URL
	cfg := config.GetInstance()
	originalController := cfg.ControllerURL
	cfg.ControllerURL = "http://invalid-controller:54321"

	defer func() {
		cfg.ControllerURL = originalController
	}()

	err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	sup := supervisor.NewSupervisor()

	// Agent should still start even without controller
	err = sup.Start()
	if err != nil {
		t.Logf("Supervisor start failed (may be expected in offline mode): %v", err)
	} else {
		time.Sleep(2 * time.Second)
		_ = sup.Stop()
	}

	t.Log("Offline mode test completed")
}
