package dnsresolver

import (
	"context"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
)

func TestReconcileAgainstRecordsRepairsLossDriftAndStaleIndex(t *testing.T) {
	r := newTestResolver()
	r.UpsertWorkload(WorkloadRecord{
		UUID:        "ms-1",
		Application: "app",
		Name:        "old",
		Scope:       ScopeManaged,
		IP:          "10.0.0.2",
		Active:      false,
		StartedAt:   1,
	})
	r.UpsertWorkload(WorkloadRecord{
		UUID:        "stale-1",
		Application: "app",
		Name:        "stale",
		Scope:       ScopeManaged,
		IP:          "10.0.0.99",
		Active:      true,
		StartedAt:   2,
	})
	r.index[ScopeManaged]["ghost.alias"] = map[string]struct{}{"ghost": {}}

	report := r.reconcileAgainstRecords([]WorkloadRecord{
		{
			UUID:        "ms-1",
			Application: "app",
			Name:        "new",
			Scope:       ScopeManaged,
			IP:          "10.0.0.3",
			Active:      true,
			StartedAt:   123,
		},
		{
			UUID:        "ms-2",
			Application: "edgelet",
			Name:        "svc",
			Scope:       ScopeLocal,
			IP:          "10.0.1.2",
			Active:      true,
			StartedAt:   55,
		},
	})

	if report.added != 1 || report.updated != 1 || report.removed != 1 {
		t.Fatalf("unexpected reconcile report: %+v", report)
	}
	if len(r.workloads) != 2 {
		t.Fatalf("expected exactly 2 workloads after reconcile, got %d", len(r.workloads))
	}
	if _, ok := r.workloads["stale-1"]; ok {
		t.Fatal("stale workload should be removed")
	}
	ms1 := r.workloads["ms-1"]
	if ms1 == nil || ms1.Name != "new" || ms1.IP != "10.0.0.3" || !ms1.Active || ms1.StartedAt != 123 {
		t.Fatalf("ms-1 drift not corrected: %+v", ms1)
	}
	if _, ok := r.index[ScopeManaged]["ghost.alias"]; ok {
		t.Fatal("stale index alias should be removed during reconcile rebuild")
	}

	for scope, byName := range r.index {
		for name, ids := range byName {
			for id := range ids {
				rec, ok := r.workloads[id]
				if !ok {
					t.Fatalf("index points to missing workload id=%s name=%s scope=%s", id, name, scope)
				}
				if rec.Scope != scope {
					t.Fatalf("index scope drift for id=%s: rec=%s idx=%s", id, rec.Scope, scope)
				}
				if !containsAlias(aliasesForWorkload(*rec, r.compatOn), name) {
					t.Fatalf("index name %q is not a valid alias for workload %s", name, id)
				}
			}
		}
	}
}

func TestRunReconcileOnceUpdatesCounters(t *testing.T) {
	r := newTestResolver()
	r.SetRuntimeSnapshotProvider(func(_ context.Context) ([]WorkloadRecord, error) {
		return []WorkloadRecord{
			{
				UUID:        "ms-1",
				Application: "app",
				Name:        "svc",
				Scope:       ScopeManaged,
				IP:          "10.2.2.2",
				Active:      true,
			},
		}, nil
	})

	r.runReconcileOnce(context.Background())
	r.runReconcileOnce(context.Background())

	s := r.Snapshot()
	if s.ReconcileRunsTotal != 2 {
		t.Fatalf("expected reconcile runs=2, got %d", s.ReconcileRunsTotal)
	}
	if s.ReconcileAddTotal != 1 || s.ReconcileUpdateTotal != 0 || s.ReconcileRemoveTotal != 0 {
		t.Fatalf("unexpected reconcile correction counters: add=%d update=%d remove=%d",
			s.ReconcileAddTotal, s.ReconcileUpdateTotal, s.ReconcileRemoveTotal)
	}
}

func TestReconcileIntervalFromEnv(t *testing.T) {
	t.Setenv("EDGELET_DNS_RECONCILE_INTERVAL_SECONDS", "75")
	if got := reconcileIntervalFromEnv(); got != 75*time.Second {
		t.Fatalf("expected 75s interval, got %s", got)
	}
	t.Setenv("EDGELET_DNS_RECONCILE_INTERVAL_SECONDS", "-2")
	if got := reconcileIntervalFromEnv(); got != reconcileDefaultEvery {
		t.Fatalf("expected default interval on invalid value, got %s", got)
	}
}

func TestFilterRecordsByEnabledScopes_RemovesDisabledScopeRecords(t *testing.T) {
	r := newTestResolver()
	r.scopeEnabled[ScopeManaged] = true
	r.scopeEnabled[ScopeLocal] = false

	filtered := r.filterRecordsByEnabledScopes([]WorkloadRecord{
		{UUID: "managed-1", Scope: ScopeManaged},
		{UUID: "local-1", Scope: ScopeLocal},
	})
	if len(filtered) != 1 {
		t.Fatalf("expected only managed record, got %d", len(filtered))
	}
	if filtered[0].UUID != "managed-1" {
		t.Fatalf("unexpected filtered record %+v", filtered[0])
	}
}

func TestRunReconcileOnce_SkipsWhenNoRunningBridgeWorkloads(t *testing.T) {
	r := newTestResolver()
	r.UpsertWorkload(WorkloadRecord{
		UUID:        "existing",
		Application: "app",
		Name:        "svc",
		Scope:       ScopeManaged,
		IP:          "10.9.9.9",
		Active:      true,
	})
	r.SetRuntimeSnapshotProvider(func(_ context.Context) ([]WorkloadRecord, error) {
		return []WorkloadRecord{
			{
				UUID:        "managed-hostnet",
				Application: "app",
				Name:        "svc",
				Scope:       ScopeManaged,
				HostNetwork: true,
				Active:      true,
			},
		}, nil
	})

	r.runReconcileOnce(context.Background())

	if _, ok := r.workloads["existing"]; !ok {
		t.Fatal("expected existing workload to remain when reconcile has no eligible scope records")
	}
	s := r.Snapshot()
	if s.ReconcileRunsTotal != 1 {
		t.Fatalf("expected reconcile runs counter increment, got %d", s.ReconcileRunsTotal)
	}
}

func TestRunReconcileOnce_DisablesLocalScopeWhenWatchdogEnabled(t *testing.T) {
	r := newTestResolver()
	cfg := config.GetInstance()
	origWatchdog := cfg.WatchdogEnabled
	cfg.WatchdogEnabled = true
	t.Cleanup(func() { cfg.WatchdogEnabled = origWatchdog })

	r.SetRuntimeSnapshotProvider(func(_ context.Context) ([]WorkloadRecord, error) {
		return []WorkloadRecord{
			{
				UUID:        "local-1",
				Application: "edgelet",
				Name:        "svc",
				Scope:       ScopeLocal,
				IP:          "10.8.8.8",
				HostNetwork: false,
				Active:      true,
			},
		}, nil
	})

	r.runReconcileOnce(context.Background())

	if r.isScopeEnabled(ScopeLocal) {
		t.Fatal("expected local scope disabled with watchdog enabled")
	}
	if _, ok := r.workloads["local-1"]; ok {
		t.Fatal("expected local workload not reconciled when local scope disabled by watchdog")
	}
}

func containsAlias(aliases []string, target string) bool {
	for _, a := range aliases {
		if a == target {
			return true
		}
	}
	return false
}
