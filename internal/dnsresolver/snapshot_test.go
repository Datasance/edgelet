package dnsresolver

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadSnapshotRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot-v1.json")

	payload := resolverSnapshotV1{
		Version:     snapshotVersionV1,
		GeneratedAt: 12345,
		Workloads: []WorkloadRecord{
			{
				UUID:        "ms-1",
				Application: "app",
				Name:        "svc",
				Scope:       ScopeManaged,
				IP:          "10.0.0.2",
				Active:      true,
				StartedAt:   99,
			},
		},
	}
	if err := saveSnapshotAtomic(path, payload); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	records, err := loadSnapshotRecords(path)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].UUID != "ms-1" || records[0].Scope != ScopeManaged || !records[0].Active {
		t.Fatalf("unexpected roundtrip record: %+v", records[0])
	}
}

func TestLoadSnapshotCorruptFailsSafe(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot-v1.json")
	if err := os.WriteFile(path, []byte("{invalid-json"), 0o644); err != nil {
		t.Fatalf("write corrupt snapshot: %v", err)
	}
	if _, err := loadSnapshotRecords(path); err == nil {
		t.Fatalf("expected unmarshal error for corrupt snapshot")
	}
}

func TestRestoreSnapshotMissingFileDoesNotFail(t *testing.T) {
	t.Parallel()
	r := newTestResolver()
	r.snapshotPath = filepath.Join(t.TempDir(), "missing.json")
	if err := r.restoreSnapshot(); err != nil {
		t.Fatalf("restore should tolerate missing file, got err=%v", err)
	}
}

func TestApplySnapshotRebuildsIndexDeterministically(t *testing.T) {
	t.Parallel()
	r := newTestResolver()
	r.UpsertWorkload(WorkloadRecord{
		UUID:        "stale",
		Application: "app",
		Name:        "old",
		Scope:       ScopeManaged,
		IP:          "10.0.0.9",
		Active:      true,
	})
	r.index[ScopeManaged]["ghost.alias"] = map[string]struct{}{"ghost": {}}

	restored := r.applySnapshotRecords([]WorkloadRecord{
		{
			UUID:        "ms-1",
			Application: "app",
			Name:        "svc",
			Scope:       ScopeManaged,
			IP:          "10.0.0.2",
			Active:      true,
			StartedAt:   100,
		},
		{
			UUID:        "ms-2",
			Application: "edgelet",
			Name:        "svc",
			Scope:       ScopeLocal,
			IP:          "10.0.1.2",
			Active:      false,
			StartedAt:   200,
		},
	})

	if restored != 2 {
		t.Fatalf("expected restored count=2, got %d", restored)
	}
	if _, ok := r.workloads["stale"]; ok {
		t.Fatalf("stale record should be replaced on restore")
	}
	if _, ok := r.index[ScopeManaged]["ghost.alias"]; ok {
		t.Fatalf("stale index alias should be removed")
	}
	if _, ok := r.index[ScopeManaged]["app.svc"]; !ok {
		t.Fatalf("expected managed alias app.svc")
	}
	if _, ok := r.index[ScopeLocal]["edgelet.svc"]; !ok {
		t.Fatalf("expected local alias edgelet.svc")
	}
}

func TestPersistSnapshotIfNeededSkipsRedundantWrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot-v1.json")

	r := newTestResolver()
	r.snapshotPath = path
	r.UpsertWorkload(WorkloadRecord{
		UUID:        "ms-1",
		Application: "app",
		Name:        "svc",
		Scope:       ScopeManaged,
		IP:          "10.0.0.2",
		Active:      true,
	})
	r.snapshotRevision.Store(1)
	r.snapshotPersisted.Store(0)

	r.persistSnapshotIfNeeded(false)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read first snapshot: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	r.persistSnapshotIfNeeded(false)
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read second snapshot: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("snapshot should not be rewritten when unchanged")
	}
}
