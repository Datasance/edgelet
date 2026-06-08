package dnsresolver

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	defaultSnapshotPath = "/var/lib/edgelet/dns/snapshot-v1.json"
	snapshotVersionV1   = "v1"
)

type resolverSnapshotV1 struct {
	Version     string           `json:"version"`
	GeneratedAt int64            `json:"generatedAt"`
	Workloads   []WorkloadRecord `json:"workloads"`
}

func (r *Resolver) snapshotLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.snapshotEvery)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			r.persistSnapshotIfNeeded(true)
			return
		case <-ticker.C:
			r.persistSnapshotIfNeeded(false)
		case <-r.snapshotTriggerCh:
			r.persistSnapshotIfNeeded(false)
		}
	}
}

func (r *Resolver) triggerSnapshotPersist() {
	r.mu.RLock()
	ch := r.snapshotTriggerCh
	r.mu.RUnlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (r *Resolver) restoreSnapshot() error {
	records, err := loadSnapshotRecords(r.snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	restored := r.applySnapshotRecords(records)
	if restored > 0 {
		logging.LogInfo(moduleName, fmt.Sprintf("dns snapshot restored records=%d path=%s", restored, r.snapshotPath))
	}
	return nil
}

func (r *Resolver) applySnapshotRecords(records []WorkloadRecord) int {
	normalized := make(map[string]WorkloadRecord, len(records))
	for _, rec := range records {
		n, ok := r.normalizeReconcileRecord(rec)
		if !ok {
			continue
		}
		normalized[n.UUID] = n
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for uuid := range r.workloads {
		delete(r.workloads, uuid)
	}
	for uuid, rec := range normalized {
		recCopy := rec
		r.workloads[uuid] = &recCopy
	}
	r.rebuildIndexLocked()

	rev := uint64(0)
	if len(normalized) > 0 {
		rev = 1
	}
	r.snapshotRevision.Store(rev)
	r.snapshotPersisted.Store(rev)
	return len(normalized)
}

func (r *Resolver) persistSnapshotIfNeeded(force bool) {
	rev := r.snapshotRevision.Load()
	if !force && rev == r.snapshotPersisted.Load() {
		return
	}

	records := r.collectSnapshotWorkloads()
	payload := resolverSnapshotV1{
		Version:     snapshotVersionV1,
		GeneratedAt: time.Now().UnixMilli(),
		Workloads:   records,
	}
	if err := saveSnapshotAtomic(r.snapshotPath, payload); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("dns snapshot persist failed path=%s err=%v", r.snapshotPath, err))
		return
	}
	r.snapshotPersisted.Store(rev)
}

func (r *Resolver) collectSnapshotWorkloads() []WorkloadRecord {
	r.mu.RLock()
	out := make([]WorkloadRecord, 0, len(r.workloads))
	for _, wl := range r.workloads {
		if wl == nil || strings.TrimSpace(wl.UUID) == "" {
			continue
		}
		out = append(out, *wl)
	}
	r.mu.RUnlock()
	slices.SortFunc(out, func(a, b WorkloadRecord) int {
		return cmp.Compare(a.UUID, b.UUID)
	})
	return out
}

func loadSnapshotRecords(path string) ([]WorkloadRecord, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- internal fixed path or tested temp path
	if err != nil {
		return nil, err
	}
	var payload resolverSnapshotV1
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	if payload.Version != snapshotVersionV1 {
		return nil, fmt.Errorf("unsupported snapshot version %q", payload.Version)
	}
	return payload.Workloads, nil
}

func saveSnapshotAtomic(path string, payload resolverSnapshotV1) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- resolver state dir must be traversable
		return fmt.Errorf("mkdir snapshot dir: %w", err)
	}
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644) // #nosec G304,G302 -- internal fixed path; state file group-readable
	if err != nil {
		return fmt.Errorf("open temp snapshot: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode snapshot: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temp snapshot: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp snapshot: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}
	syncDirBestEffort(filepath.Dir(path))
	return nil
}

func syncDirBestEffort(path string) {
	d, err := os.Open(path) // #nosec G304 -- internal fixed snapshot directory
	if err != nil {
		return
	}
	defer func() {
		_ = d.Close()
	}()
	_ = d.Sync()
}
