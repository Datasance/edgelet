package store

import "fmt"

// ClearControllerMicroserviceRuntimeFields clears ephemeral runtime columns while keeping spec rows.
func (d *DB) ClearControllerMicroserviceRuntimeFields() error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec(`UPDATE controller_microservices SET
		container_id = '',
		container_ip = NULL,
		is_stuck_in_restart = 0,
		is_updating = 0`)
	if err != nil {
		return fmt.Errorf("clear controller microservice runtime fields: %w", err)
	}
	return nil
}

// ClearLocalWorkloadRuntimeFields clears runtime tracking on local workload rows.
func (d *DB) ClearLocalWorkloadRuntimeFields() error {
	if d.Conn() == nil {
		return nil
	}
	_, err := d.Conn().Exec(`UPDATE local_workloads SET
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
		return fmt.Errorf("clear local workload runtime fields: %w", err)
	}
	return nil
}
