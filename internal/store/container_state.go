package store

import (
	"database/sql"
	"fmt"
)

// ContainerState holds the workload and sandbox container IDs for a microservice.
// Used by the iofog engine for fast lookup; Docker/Podman do not populate this.
type ContainerState struct {
	MsUUID     string
	WorkloadID string
	SandboxID  string
}

// SaveContainerState upserts a container state row for the given microservice.
func (d *DB) SaveContainerState(msUUID, workloadID, sandboxID string) error {
	if d.Conn() == nil {
		return nil // DB not opened yet
	}
	_, err := d.Conn().Exec(
		`INSERT INTO container_state (ms_uuid, workload_id, sandbox_id, updated_at)
		 VALUES (?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(ms_uuid) DO UPDATE SET
		   workload_id = excluded.workload_id,
		   sandbox_id = excluded.sandbox_id,
		   updated_at = strftime('%s','now')`,
		msUUID, workloadID, sandboxID,
	)
	return err
}

// GetContainerState returns the container state for the given microservice UUID.
// Returns nil if not found or DB not opened.
func (d *DB) GetContainerState(msUUID string) (*ContainerState, error) {
	if d.Conn() == nil {
		return nil, nil
	}
	var cs ContainerState
	err := d.Conn().QueryRow(
		`SELECT ms_uuid, workload_id, sandbox_id FROM container_state WHERE ms_uuid = ?`,
		msUUID,
	).Scan(&cs.MsUUID, &cs.WorkloadID, &cs.SandboxID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get container state: %w", err)
	}
	return &cs, nil
}

// DeleteContainerState removes the container state row for the given microservice.
func (d *DB) DeleteContainerState(msUUID string) error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec("DELETE FROM container_state WHERE ms_uuid = ?", msUUID)
	return err
}
