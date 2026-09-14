package modelcatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
)

func TestPrepare_StartGateStatesAndRefs(t *testing.T) {
	db := openCatalogTestDB(t)
	disk := t.TempDir()
	content := filepath.Join(disk, "models", "test-model", "content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(content, "weights.bin"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	ms := models.NewMicroservice("ms-gate", "nginx:latest")
	ms.Models = &models.ModelCatalog{
		BindPath: "/models",
		Items:    []models.ModelCatalogItem{{Name: "test-model"}},
	}

	upsertModel(t, db, "test-model", models.ModelSourceLocal, models.ModelStatePulling, content, 1)
	waiting, err := Prepare(disk, db, ms, models.ModelSourceLocal, true)
	if err != nil {
		t.Fatalf("prepare pulling: %v", err)
	}
	if waiting.Decision != models.CatalogGateWait {
		t.Fatalf("expected wait, got %v", waiting.Decision)
	}
	if !strings.Contains(waiting.Message, "test-model") || !strings.Contains(waiting.Message, models.ModelStatePulling) {
		t.Fatalf("expected wait text with model name and state, got %q", waiting.Message)
	}
	assertRefCount(t, db, "test-model", 1)
	if _, err := os.Stat(filepath.Join(waiting.HostDir, "test-model")); !os.IsNotExist(err) {
		t.Fatal("must not project until Ready")
	}

	upsertModel(t, db, "test-model", models.ModelSourceLocal, models.ModelStateFailed, content, 1)
	row, err := db.GetLocalModel("test-model")
	if err != nil {
		t.Fatal(err)
	}
	row.LastError = "download failed"
	if err := db.UpsertLocalModel(row); err != nil {
		t.Fatal(err)
	}
	failed, err := Prepare(disk, db, ms, models.ModelSourceLocal, true)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if failed.Decision != models.CatalogGateFail {
		t.Fatalf("expected fail, got %v", failed.Decision)
	}

	upsertModel(t, db, "test-model", models.ModelSourceLocal, models.ModelStateReady, content, 2)
	ready, err := Prepare(disk, db, ms, models.ModelSourceLocal, true)
	if err != nil {
		t.Fatalf("prepare ready: %v", err)
	}
	if ready.Decision != models.CatalogGateAllow {
		t.Fatalf("expected allow, got %v msg=%q", ready.Decision, ready.Message)
	}
	if _, err := os.ReadFile(filepath.Join(ready.HostDir, "test-model", "weights.bin")); err != nil { // #nosec G304 -- test fixture
		t.Fatalf("expected projected content: %v", err)
	}

	if err := Release(disk, db, ms.MicroserviceUUID); err != nil {
		t.Fatalf("release: %v", err)
	}
	assertRefCount(t, db, "test-model", 0)
}

func TestPrepare_SourceScopedBind(t *testing.T) {
	db := openCatalogTestDB(t)
	disk := t.TempDir()
	upsertModel(t, db, "fleet-model", models.ModelSourceManaged, models.ModelStateReady, "", 1)

	ms := models.NewMicroservice("ms-local", "nginx:latest")
	ms.Models = &models.ModelCatalog{
		BindPath: "/models",
		Items:    []models.ModelCatalogItem{{Name: "fleet-model"}},
	}
	res, err := Prepare(disk, db, ms, models.ModelSourceLocal, true)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if res.Decision != models.CatalogGateFail {
		t.Fatalf("local workload must not bind managed names, got %v", res.Decision)
	}
	assertRefCount(t, db, "fleet-model", 0)
}

func openCatalogTestDB(t *testing.T) *store.DB {
	t.Helper()
	db := store.GetInstance()
	_ = db.Close()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func upsertModel(t *testing.T, db *store.DB, name, source, state, content string, generation int64) {
	t.Helper()
	row := &models.LocalModel{
		Name:        name,
		Source:      source,
		Repo:        "org/repo",
		RegistryID:  1,
		State:       state,
		ContentPath: content,
		Generation:  generation,
		LastError:   "",
	}
	row.NormalizeDefaults()
	row.State = state
	row.Source = source
	row.Generation = generation
	if err := db.UpsertLocalModel(row); err != nil {
		t.Fatalf("upsert model %s: %v", name, err)
	}
}

func assertRefCount(t *testing.T, db *store.DB, name string, want int) {
	t.Helper()
	n, err := db.CountModelRefs(name)
	if err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("ref count for %s: got %d want %d", name, n, want)
	}
}
