package store

import "fmt"

// ClearAllContainerStates removes all managed microservice container_state rows.
func (d *DB) ClearAllContainerStates() error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec("DELETE FROM container_state")
	if err != nil {
		return fmt.Errorf("clear container_state: %w", err)
	}
	return nil
}

// ClearMicroserviceRuntimeFields clears ephemeral runtime columns while keeping spec rows.
func (d *DB) ClearMicroserviceRuntimeFields() error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec(`UPDATE microservices SET
		container_id = '',
		container_ip = NULL,
		is_stuck_in_restart = 0,
		is_updating = 0`)
	if err != nil {
		return fmt.Errorf("clear microservice runtime fields: %w", err)
	}
	return nil
}

// ClearLocalDeployedRuntimeFields clears runtime tracking on local deployment rows.
func (d *DB) ClearLocalDeployedRuntimeFields() error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec(`UPDATE local_deployed_microservices SET
		container_id = '',
		state = 'unknown',
		runtime_state = 'unknown',
		last_error = '',
		restart_count = 0,
		last_transition_at = 0,
		last_reconcile_at = 0,
		last_start_attempt_at = 0,
		failure_count = 0`)
	if err != nil {
		return fmt.Errorf("clear local deployed runtime fields: %w", err)
	}
	return nil
}
