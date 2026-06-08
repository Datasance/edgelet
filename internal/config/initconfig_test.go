package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitConfigCreatesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	created, err := InitConfig(path)
	if err != nil {
		t.Fatalf("InitConfig: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty config")
	}

	created, err = InitConfig(path)
	if err != nil {
		t.Fatalf("InitConfig second call: %v", err)
	}
	if created {
		t.Fatal("expected created=false when config exists")
	}
}

func TestInitConfigIdempotentPreservesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("currentProfile: custom\n"), 0600); err != nil {
		t.Fatal(err)
	}

	created, err := InitConfig(path)
	if err != nil {
		t.Fatalf("InitConfig: %v", err)
	}
	if created {
		t.Fatal("expected created=false for existing file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "currentProfile: custom\n" {
		t.Fatalf("config was modified: %q", string(data))
	}
}
