package store

import "testing"

func TestEnsureDefaultControllerRegistries(t *testing.T) {
	db := GetInstance()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.EnsureDefaultControllerRegistries(); err != nil {
		t.Fatalf("EnsureDefaultControllerRegistries failed: %v", err)
	}
	registries, err := db.LoadControllerRegistries()
	if err != nil {
		t.Fatalf("LoadControllerRegistries failed: %v", err)
	}

	foundDockerIO := false
	foundFromCache := false
	for _, reg := range registries {
		if reg.ID == 1 && reg.URL == "docker.io" {
			foundDockerIO = true
		}
		if reg.ID == 2 && reg.URL == "from_cache" {
			foundFromCache = true
		}
	}
	if !foundDockerIO || !foundFromCache {
		t.Fatalf("expected default registries to exist, got %+v", registries)
	}
}
