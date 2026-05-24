package supervisor

import (
	"testing"

	"github.com/datasance/edgelet/internal/store"
)

func TestEnsureDefaultLocalRegistriesOnStartup_SeedsDefaults(t *testing.T) {
	db := store.GetInstance()
	_ = db.Close()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := NewSupervisor()
	if err := s.ensureDefaultLocalRegistriesOnStartup(db); err != nil {
		t.Fatalf("ensureDefaultLocalRegistriesOnStartup failed: %v", err)
	}

	registries, err := db.LoadLocalRegistries()
	if err != nil {
		t.Fatalf("LoadLocalRegistries failed: %v", err)
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
		t.Fatalf("expected default local registries to exist, got %+v", registries)
	}
}
