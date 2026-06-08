package config

import (
	"os"
	"testing"
)

func TestSetConfigForcesEdgeGuardDisabledWhenUnprovisioned(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "setconfig-invariant-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testConfig := `currentProfile: default
profiles:
  default:
    iofogUuid: ""
    edgeGuardFrequency: "0"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg := GetInstance()
	errors := cfg.SetConfig(map[string]any{"egf": 10})
	if len(errors) > 0 {
		t.Fatalf("expected no setconfig error, got: %+v", errors)
	}
	if cfg.EdgeGuardFrequency != 0 {
		t.Fatalf("expected edgeGuardFrequency forced to 0 for unprovisioned agent, got %d", cfg.EdgeGuardFrequency)
	}
}

func TestSetConfigWatchdogEnabledAcceptsFalseSynonyms(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "setconfig-watchdog-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testConfig := `currentProfile: default
profiles:
  default:
    watchdogEnabled: "on"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg := GetInstance()

	for _, value := range []any{"false", "0", "no"} {
		cfg.WatchdogEnabled = true
		errors := cfg.SetConfig(map[string]any{"wd": value})
		if len(errors) > 0 {
			t.Fatalf("expected no setconfig error for value=%v, got: %+v", value, errors)
		}
		if cfg.WatchdogEnabled {
			t.Fatalf("expected watchdog disabled for value=%v", value)
		}
	}
}
