package dnsresolver

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

// RuntimeSnapshotProvider returns runtime-truth workload records for reconcile.
type RuntimeSnapshotProvider func(ctx context.Context) ([]WorkloadRecord, error)

type reconcileReport struct {
	added   int
	updated int
	removed int
}

func reconcileIntervalFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("IOFOG_DNS_RECONCILE_INTERVAL_SECONDS"))
	if raw == "" {
		return reconcileDefaultEvery
	}
	secs, err := strconv.Atoi(raw)
	if err != nil || secs <= 0 {
		logging.LogWarn(moduleName, fmt.Sprintf("invalid IOFOG_DNS_RECONCILE_INTERVAL_SECONDS=%q, using default=%ds", raw, int(reconcileDefaultEvery/time.Second)))
		return reconcileDefaultEvery
	}
	return time.Duration(secs) * time.Second
}

func (r *Resolver) reconcileLoop() {
	defer r.wg.Done()

	r.runReconcileOnce(context.Background())

	r.mu.RLock()
	interval := r.reconcileEvery
	r.mu.RUnlock()
	if interval <= 0 {
		interval = reconcileDefaultEvery
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.runReconcileOnce(context.Background())
		}
	}
}

func (r *Resolver) runReconcileOnce(ctx context.Context) {
	r.mu.RLock()
	provider := r.reconcileProvider
	r.mu.RUnlock()
	if provider == nil {
		return
	}

	records, err := provider(ctx)
	if err != nil {
		r.reconcileErrors.Add(1)
		logging.LogWarn(moduleName, fmt.Sprintf("dns reconcile snapshot read failed: %v", err))
		return
	}
	r.updateScopePolicy(records)
	filteredRecords := r.filterRecordsByEnabledScopes(records)
	if len(filteredRecords) == 0 {
		r.reconcileRuns.Add(1)
		logging.LogDebug(moduleName, "dns reconcile skipped: no eligible scope records")
		return
	}

	report := r.reconcileAgainstRecords(filteredRecords)
	r.reconcileRuns.Add(1)
	if report.added > 0 || report.updated > 0 || report.removed > 0 {
		logging.LogInfo(
			moduleName,
			fmt.Sprintf("dns reconcile corrections applied add=%d update=%d remove=%d decision=converged", report.added, report.updated, report.removed),
		)
		return
	}
	logging.LogDebug(moduleName, "dns reconcile run completed with no corrections")
}

func (r *Resolver) reconcileAgainstRecords(records []WorkloadRecord) reconcileReport {
	desired := make(map[string]WorkloadRecord, len(records))
	for _, rec := range records {
		if normalized, ok := r.normalizeReconcileRecord(rec); ok {
			desired[normalized.UUID] = normalized
		}
	}

	report := reconcileReport{}

	r.mu.Lock()
	defer r.mu.Unlock()

	for uuid, existing := range r.workloads {
		if _, ok := desired[uuid]; ok {
			continue
		}
		r.deindexLocked(existing)
		delete(r.workloads, uuid)
		report.removed++
	}

	for uuid, desiredRec := range desired {
		current, exists := r.workloads[uuid]
		if !exists {
			recCopy := desiredRec
			r.workloads[uuid] = &recCopy
			report.added++
			continue
		}
		if workloadEqual(*current, desiredRec) {
			continue
		}
		recCopy := desiredRec
		r.workloads[uuid] = &recCopy
		report.updated++
	}

	r.rebuildIndexLocked()

	if report.added > 0 {
		r.reconcileAdds.Add(uint64(report.added))
	}
	if report.updated > 0 {
		r.reconcileUpdates.Add(uint64(report.updated))
	}
	if report.removed > 0 {
		r.reconcileRemoves.Add(uint64(report.removed))
	}

	return report
}

func (r *Resolver) rebuildIndexLocked() {
	r.index = map[Scope]map[string]map[string]struct{}{
		ScopeManaged: make(map[string]map[string]struct{}),
		ScopeLocal:   make(map[string]map[string]struct{}),
	}
	for _, rec := range r.workloads {
		if rec == nil {
			continue
		}
		r.indexLocked(rec)
	}
}

func (r *Resolver) normalizeReconcileRecord(rec WorkloadRecord) (WorkloadRecord, bool) {
	rec.UUID = strings.TrimSpace(rec.UUID)
	if rec.UUID == "" {
		return WorkloadRecord{}, false
	}
	rec.Application = strings.TrimSpace(rec.Application)
	rec.Name = strings.TrimSpace(rec.Name)
	rec.IP = strings.TrimSpace(rec.IP)
	rec.Scope = normalizeScope(rec.Scope)
	if rec.HostNetwork && rec.IP == "" {
		rec.IP = r.hostAdvertiseIP()
	}
	return rec, true
}

func workloadEqual(a, b WorkloadRecord) bool {
	return a.UUID == b.UUID &&
		a.Application == b.Application &&
		a.Name == b.Name &&
		a.Scope == b.Scope &&
		a.IP == b.IP &&
		a.HostNetwork == b.HostNetwork &&
		a.IsRouter == b.IsRouter &&
		a.IsNats == b.IsNats &&
		a.Active == b.Active &&
		a.StartedAt == b.StartedAt
}
