package config

import (
	"testing"
)

func TestTriggerGPSConfigCallback_NoCallback(t *testing.T) {
	cfg := GetInstance()
	cfg.SetGPSConfigCallback(nil)

	if err := cfg.TriggerGPSConfigCallback(); err != nil {
		t.Fatalf("expected nil error when callback is unset, got %v", err)
	}
}

func TestTriggerGPSConfigCallback_InvokesCallback(t *testing.T) {
	cfg := GetInstance()
	called := false
	cfg.SetGPSConfigCallback(func() error {
		called = true
		return nil
	})

	if err := cfg.TriggerGPSConfigCallback(); err != nil {
		t.Fatalf("expected nil error when callback succeeds, got %v", err)
	}
	if !called {
		t.Fatal("expected GPS config callback to be invoked")
	}
}
