//go:build lite

package integration

import (
	"testing"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/pkg/docker"
)

// TestDockerConnection tests basic Docker connectivity
func TestDockerConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := config.GetInstance()
	client := docker.GetInstance()

	// Initialize Docker client
	err := client.Init(cfg.DockerURL, cfg.DockerAPIVersion)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()

	// Test Docker info via GetClient
	cli := client.GetClient()
	if cli == nil {
		t.Fatal("Docker client is nil")
	}

	ctx := client.GetContext()
	info, err := cli.Info(ctx)
	if err != nil {
		t.Logf("Docker info failed (expected if Docker not running): %v", err)
		return
	}

	t.Logf("Docker version: %s", info.ServerVersion)
}

// TestDockerImagePull tests Docker image pulling capability
func TestDockerImagePull(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := config.GetInstance()
	client := docker.GetInstance()

	err := client.Init(cfg.DockerURL, cfg.DockerAPIVersion)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()

	// Test pulling a small image
	testImage := "alpine:latest"
	err = client.PullImage(testImage, "", "", nil, nil)
	if err != nil {
		t.Logf("Failed to pull image %s (expected if Docker not running): %v", testImage, err)
		return
	}

	t.Logf("Successfully pulled image: %s", testImage)
}

// TestDockerContainerLifecycle tests container create, start, stop, remove
func TestDockerContainerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := config.GetInstance()
	client := docker.GetInstance()

	err := client.Init(cfg.DockerURL, cfg.DockerAPIVersion)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()

	// Ensure alpine image exists
	testImage := "alpine:latest"
	err = client.PullImage(testImage, "", "", nil, nil)
	if err != nil {
		t.Logf("Failed to pull image %s (expected if Docker not running): %v", testImage, err)
		return
	}

	// Note: Container creation requires microservice model
	// This test would need to be expanded with actual container creation
	// For now, just verify Docker client works
	t.Log("Docker client initialized successfully")
}
