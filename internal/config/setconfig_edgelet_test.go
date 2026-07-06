package config

import (
	"os"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/constants"
)

func TestSetConfigContainerEngineURLNoOpForEdgeletEngine(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "setconfig-edgelet-cu-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	want := constants.EdgeletEngineSocketURL()
	testConfig := `currentProfile: default
profiles:
  default:
    containerEngine: edgelet
    containerEngineUrl: "` + want + `"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg := GetInstance()
	errors := cfg.SetConfig(map[string]any{"cu": want})
	if len(errors) > 0 {
		t.Fatalf("expected no error echoing fixed edgelet socket, got: %+v", errors)
	}
	if cfg.ContainerEngineURL != want {
		t.Fatalf("ContainerEngineURL=%q want %q", cfg.ContainerEngineURL, want)
	}
}

func TestSetConfigContainerEngineURLRejectsWrongValueForEdgeletEngine(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "setconfig-edgelet-cu-reject-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	want := constants.EdgeletEngineSocketURL()
	testConfig := `currentProfile: default
profiles:
  default:
    containerEngine: edgelet
    containerEngineUrl: "` + want + `"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg := GetInstance()
	errors := cfg.SetConfig(map[string]any{"cu": "unix:///var/run/docker.sock"})
	if len(errors) == 0 {
		t.Fatal("expected error when changing containerEngineUrl on edgelet engine")
	}
	if got := errors["cu"]; got == "" {
		t.Fatalf("expected cu error, got: %+v", errors)
	}
	if cfg.ContainerEngineURL != want {
		t.Fatalf("ContainerEngineURL=%q want unchanged %q", cfg.ContainerEngineURL, want)
	}
}

func TestSetConfigAppliesContainerEngineBeforeURL(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "setconfig-ce-before-cu-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	edgeletURL := constants.EdgeletEngineSocketURL()
	dockerURL := "unix:///var/run/docker.sock"
	testConfig := `currentProfile: default
profiles:
  default:
    containerEngine: edgelet
    containerEngineUrl: "` + edgeletURL + `"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg := GetInstance()
	// Map iteration order is random; ce must still be applied before cu.
	errors := cfg.SetConfig(map[string]any{
		"cu": dockerURL,
		"ce": constants.EngineDocker,
	})
	if len(errors) > 0 {
		t.Fatalf("expected engine+url batch to succeed, got: %+v", errors)
	}
	if cfg.ContainerEngine != constants.EngineDocker {
		t.Fatalf("ContainerEngine=%q want docker", cfg.ContainerEngine)
	}
	if cfg.ContainerEngineURL != dockerURL {
		t.Fatalf("ContainerEngineURL=%q want %q", cfg.ContainerEngineURL, dockerURL)
	}
}
