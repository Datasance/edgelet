package store

import (
	"database/sql"
	"fmt"
)

// Runtime container ref scopes stored in runtime_container_refs.scope.
const (
	RuntimeScopeController = "controller"
	RuntimeScopeLocal      = "local"
)

// ContainerState holds workload and sandbox container IDs for a microservice at a scope.
type ContainerState struct {
	MsUUID     string
	Scope      string
	WorkloadID string
	SandboxID  string
}

// UpsertRuntimeContainerRef upserts a runtime container ref row.
func (d *DB) UpsertRuntimeContainerRef(msUUID, scope, workloadID, sandboxID string) error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec(
		`INSERT INTO runtime_container_refs (ms_uuid, scope, workload_id, sandbox_id, updated_at)
		 VALUES (?, ?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(ms_uuid, scope) DO UPDATE SET
		   workload_id = excluded.workload_id,
		   sandbox_id = excluded.sandbox_id,
		   updated_at = strftime('%s','now')`,
		msUUID, scope, workloadID, sandboxID,
	)
	return err
}

// GetRuntimeContainerRef returns the runtime container ref for msUUID at scope.
// Returns nil if not found or DB not opened.
func (d *DB) GetRuntimeContainerRef(msUUID, scope string) (*ContainerState, error) {
	if d.Conn() == nil {
		return nil, nil
	}
	var cs ContainerState
	err := d.Conn().QueryRow(
		`SELECT ms_uuid, scope, workload_id, sandbox_id FROM runtime_container_refs WHERE ms_uuid = ? AND scope = ?`,
		msUUID, scope,
	).Scan(&cs.MsUUID, &cs.Scope, &cs.WorkloadID, &cs.SandboxID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get runtime container ref: %w", err)
	}
	return &cs, nil
}

// DeleteRuntimeContainerRef removes the runtime container ref for msUUID at scope.
func (d *DB) DeleteRuntimeContainerRef(msUUID, scope string) error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec("DELETE FROM runtime_container_refs WHERE ms_uuid = ? AND scope = ?", msUUID, scope)
	return err
}

// ClearRuntimeContainerRefs removes runtime container ref rows.
// When scope is non-empty, only rows matching that scope are deleted.
func (d *DB) ClearRuntimeContainerRefs(scope string) error {
	if d.Conn() == nil {
		return nil
	}
	var err error
	if scope == "" {
		_, err = d.Conn().Exec("DELETE FROM runtime_container_refs")
	} else {
		_, err = d.Conn().Exec("DELETE FROM runtime_container_refs WHERE scope = ?", scope)
	}
	if err != nil {
		return fmt.Errorf("clear runtime_container_refs: %w", err)
	}
	return nil
}
