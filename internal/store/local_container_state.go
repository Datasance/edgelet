package store

import (
	"database/sql"
	"fmt"
)

// SaveLocalContainerState upserts a local workload state row for the given microservice.
func (d *DB) SaveLocalContainerState(msUUID, workloadID, sandboxID string) error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec(
		`INSERT INTO local_container_state (ms_uuid, workload_id, sandbox_id, updated_at)
		 VALUES (?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(ms_uuid) DO UPDATE SET
		   workload_id = excluded.workload_id,
		   sandbox_id = excluded.sandbox_id,
		   updated_at = strftime('%s','now')`,
		msUUID, workloadID, sandboxID,
	)
	return err
}

// GetLocalContainerState returns local workload state for a microservice.
func (d *DB) GetLocalContainerState(msUUID string) (*ContainerState, error) {
	if d.Conn() == nil {
		return nil, nil
	}
	var cs ContainerState
	err := d.Conn().QueryRow(
		`SELECT ms_uuid, workload_id, sandbox_id FROM local_container_state WHERE ms_uuid = ?`,
		msUUID,
	).Scan(&cs.MsUUID, &cs.WorkloadID, &cs.SandboxID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get local container state: %w", err)
	}
	return &cs, nil
}

// DeleteLocalContainerState removes local container state by microservice id.
func (d *DB) DeleteLocalContainerState(msUUID string) error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec("DELETE FROM local_container_state WHERE ms_uuid = ?", msUUID)
	return err
}

// ClearLocalContainerStates removes all local container state rows.
func (d *DB) ClearLocalContainerStates() error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec("DELETE FROM local_container_state")
	return err
}
