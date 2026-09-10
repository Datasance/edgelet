package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestLocalRegistryUpsertBySpecID(t *testing.T) {
	db := openFreshStoreDB(t)

	reg := models.NewRegistryBuilder().
		SetID(5).
		SetURL("quay.io").
		SetIsPublic(false).
		SetUserName("john").
		SetPassword("s3cr3t").
		SetType(models.RegistryTypeOCI).
		Build()
	if err := db.UpsertLocalRegistry(reg); err != nil {
		t.Fatalf("upsert id 5: %v", err)
	}

	got, err := db.GetLocalRegistry(5)
	if err != nil {
		t.Fatalf("get id 5: %v", err)
	}
	if got.URL != "quay.io" || got.Type != models.RegistryTypeOCI || got.UserName != "john" {
		t.Fatalf("unexpected registry: %+v", got)
	}

	update := models.NewRegistryBuilder().
		SetID(5).
		SetURL("quay.io").
		SetIsPublic(false).
		SetUserName("john").
		SetPassword("rotated").
		SetType(models.RegistryTypeOCI).
		SetCAB64("Y2E=").
		SetInsecure(true).
		Build()
	if err := db.UpsertLocalRegistry(update); err != nil {
		t.Fatalf("upsert same id same identity: %v", err)
	}
	got, err = db.GetLocalRegistry(5)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Password != "rotated" || got.CAB64 != "Y2E=" || !got.Insecure {
		t.Fatalf("expected in-place credential/tls update, got %+v", got)
	}

	collide := models.NewRegistryBuilder().
		SetID(5).
		SetURL("ghcr.io").
		SetType(models.RegistryTypeOCI).
		Build()
	err = db.UpsertLocalRegistry(collide)
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("expected collision on different url, got: %v", err)
	}
}

func TestNextLocalRegistryID(t *testing.T) {
	db := openFreshStoreDB(t)
	if err := db.EnsureDefaultLocalRegistries(); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	id, err := db.NextLocalRegistryID()
	if err != nil {
		t.Fatalf("next id: %v", err)
	}
	if id != 4 {
		t.Fatalf("expected next id 4 after built-in registries, got %d", id)
	}

	reg := models.NewRegistry(9, "registry.example.com", true, "", "", "")
	if err := db.UpsertLocalRegistry(reg); err != nil {
		t.Fatalf("upsert 9: %v", err)
	}
	id, err = db.NextLocalRegistryID()
	if err != nil {
		t.Fatalf("next id after 9: %v", err)
	}
	if id != 10 {
		t.Fatalf("expected next id 10, got %d", id)
	}
}

func TestControllerRegistryScanIncludesTypeCAInsecure(t *testing.T) {
	db := openFreshStoreDB(t)

	hf := models.NewRegistryBuilder().
		SetID(5).
		SetURL("https://huggingface.co").
		SetIsPublic(false).
		SetPassword("hf_token").
		SetType(models.RegistryTypeHF).
		SetInsecure(true).
		Build()
	if err := db.SaveControllerRegistries([]*models.Registry{hf}); err != nil {
		t.Fatalf("save controller registries: %v", err)
	}
	got, err := db.LoadControllerRegistries()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 controller registry, got %d", len(got))
	}
	if got[0].Type != models.RegistryTypeHF || !got[0].Insecure || got[0].URL != "https://huggingface.co" {
		t.Fatalf("unexpected controller registry: %+v", got[0])
	}
}

func TestLocalModelCRUD(t *testing.T) {
	db := openFreshStoreDB(t)

	row := &models.LocalModel{
		Name:         "llama-2-7b-q2k",
		Repo:         "second-state/Llama-2-7B-Chat-GGUF",
		Revision:     "064fe43ea8c1e1f93477ef4a170bdc2b244ef02c",
		RegistryID:   5,
		Format:       models.ModelFormatGGUF,
		ManifestYAML: "kind: Model",
		ManifestPath: "/var/lib/edgelet/models/llama-2-7b-q2k/manifest.json",
		ContentPath:  "/var/lib/edgelet/models/llama-2-7b-q2k/content",
	}
	row.SetFiles([]string{"llama-2-7b-chat.Q5_K_M.gguf"})
	if err := db.UpsertLocalModel(row); err != nil {
		t.Fatalf("upsert model: %v", err)
	}

	got, err := db.GetLocalModel("llama-2-7b-q2k")
	if err != nil {
		t.Fatalf("get model: %v", err)
	}
	if got.Repo != row.Repo || got.RegistryID != 5 || got.State != models.ModelStatePending {
		t.Fatalf("unexpected model: %+v", got)
	}
	if got.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", got.Generation)
	}
	files := got.Files()
	if len(files) != 1 || files[0] != "llama-2-7b-chat.Q5_K_M.gguf" {
		t.Fatalf("unexpected files: %v", files)
	}

	got.Revision = "main"
	got.Generation = 2
	if err := db.UpsertLocalModel(got); err != nil {
		t.Fatalf("upsert generation bump: %v", err)
	}
	updated, err := db.GetLocalModel("llama-2-7b-q2k")
	if err != nil {
		t.Fatalf("get after bump: %v", err)
	}
	if updated.Generation != 2 || updated.Revision != "main" {
		t.Fatalf("expected generation/revision update, got %+v", updated)
	}

	list, err := db.ListLocalModels()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 model, got %d", len(list))
	}

	if err := db.DeleteLocalModel("llama-2-7b-q2k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := db.GetLocalModel("llama-2-7b-q2k"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows after delete, got %v", err)
	}
}

func TestControllerModelReplaceAll(t *testing.T) {
	db := openFreshStoreDB(t)

	first := &models.ControllerModel{
		ID:         12,
		Name:       "llama-2-7b-q2k",
		Repo:       "second-state/Llama-2-7B-Chat-GGUF",
		Revision:   "064fe43ea8c1e1f93477ef4a170bdc2b244ef02c",
		RegistryID: 5,
		Format:     models.ModelFormatGGUF,
	}
	first.SetFiles([]string{"llama-2-7b-chat.Q5_K_M.gguf"})
	if err := db.SaveControllerModels([]*models.ControllerModel{first}); err != nil {
		t.Fatalf("save controller models: %v", err)
	}
	got, err := db.LoadControllerModels()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Name != first.Name || got[0].RegistryID != 5 {
		t.Fatalf("unexpected controller models: %+v", got)
	}

	replacement := &models.ControllerModel{
		ID:         13,
		Name:       "gemma3",
		Repo:       "ai/gemma3",
		RegistryID: 1,
	}
	if err := db.SaveControllerModels([]*models.ControllerModel{replacement}); err != nil {
		t.Fatalf("replace controller models: %v", err)
	}
	got, err = db.LoadControllerModels()
	if err != nil {
		t.Fatalf("load after replace: %v", err)
	}
	if len(got) != 1 || got[0].ID != 13 || got[0].Name != "gemma3" {
		t.Fatalf("expected replace-all, got %+v", got)
	}

	if err := db.ClearControllerModels(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err = db.LoadControllerModels()
	if err != nil {
		t.Fatalf("load after clear: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty controller models, got %d", len(got))
	}
}

func TestListReferencedModelNames_UnionsDeployedAndRefs(t *testing.T) {
	db := openFreshStoreDB(t)
	row := &models.LocalModel{
		Name:       "deployed",
		Repo:       "org/repo",
		RegistryID: 1,
		State:      models.ModelStateReady,
	}
	row.NormalizeDefaults()
	if err := db.UpsertLocalModel(row); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := db.Conn().Exec(
		`INSERT INTO model_refs (model_name, kind, ref_id) VALUES ('bound-only', 'workload', 'ms-1')`,
	); err != nil {
		t.Fatalf("insert ref: %v", err)
	}
	names, err := db.ListReferencedModelNames()
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	seen := map[string]bool{}
	for _, name := range names {
		seen[name] = true
	}
	if !seen["deployed"] || !seen["bound-only"] {
		t.Fatalf("expected deployed row and explicit ref, got %v", names)
	}
}
