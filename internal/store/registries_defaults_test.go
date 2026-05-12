package store

import "testing"

func TestEnsureDefaultRegistries(t *testing.T) {
	db := GetInstance()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.EnsureDefaultRegistries(); err != nil {
		t.Fatalf("EnsureDefaultRegistries failed: %v", err)
	}
	registries, err := db.LoadRegistries()
	if err != nil {
		t.Fatalf("LoadRegistries failed: %v", err)
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
