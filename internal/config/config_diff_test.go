package config

import (
	"os"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/constants"
)

func TestFilterChangedConfigKeysSkipsUnchanged(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-diff-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testConfig := `currentProfile: default
profiles:
  default:
    diskLimit: "500"
    memoryLimit: "8192"
    logLevel: "INFO"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg := GetInstance()
	changed := cfg.FilterChangedConfigKeys(map[string]any{
		"d":  500,
		"m":  8192,
		"ll": "INFO",
	})
	if len(changed) != 0 {
		t.Fatalf("expected no changed keys, got: %+v", changed)
	}
}

func TestFilterChangedConfigKeysDetectsChange(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-diff-change-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testConfig := `currentProfile: default
profiles:
  default:
    diskLimit: "500"
    changeFrequency: "20"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg := GetInstance()
	changed := cfg.FilterChangedConfigKeys(map[string]any{
		"d":  500,
		"cf": 30,
	})
	if len(changed) != 1 {
		t.Fatalf("expected one changed key, got: %+v", changed)
	}
	if _, ok := changed["cf"]; !ok {
		t.Fatalf("expected cf to be changed, got: %+v", changed)
	}
}

func TestFilterChangedConfigKeysSkipsEdgeletContainerEngineURL(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-diff-edgelet-cu-*.yaml")
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
    changeFrequency: "20"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg := GetInstance()
	changed := cfg.FilterChangedConfigKeys(map[string]any{
		"cu": want,
		"cf": 20,
	})
	if len(changed) != 0 {
		t.Fatalf("expected edgelet cu echo to be filtered, got: %+v", changed)
	}
}
