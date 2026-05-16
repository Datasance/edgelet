package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/models"
)

// UpsertLocalDeployedMicroservice upserts a locally deployed microservice record.
func (d *DB) UpsertLocalDeployedMicroservice(ms *models.LocalDeployedMicroservice) error {
	if ms == nil {
		return fmt.Errorf("local deployed microservice is nil")
	}
	if ms.LocalUUID == "" {
		return fmt.Errorf("local_uuid is required")
	}
	if ms.ManifestYAML == "" {
		return fmt.Errorf("manifest_yaml is required")
	}
	ms.NormalizeDefaults()

	_, err := d.Conn().Exec(`INSERT INTO local_deployed_microservices (
		local_uuid, application_name, microservice_name, source_name, manifest_yaml, image_name,
		state, container_id, desired_state, runtime_state, last_error, restart_count, last_transition_at, last_reconcile_at, last_start_attempt_at, failure_count,
		deleted_at, generation, observed_generation, created_at, updated_at
	) VALUES (
		?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,strftime('%s','now'),?
	)
	ON CONFLICT(local_uuid) DO UPDATE SET
		application_name=excluded.application_name,
		microservice_name=excluded.microservice_name,
		source_name=excluded.source_name,
		manifest_yaml=excluded.manifest_yaml,
		image_name=excluded.image_name,
		state=excluded.state,
		container_id=excluded.container_id,
		desired_state=excluded.desired_state,
		runtime_state=excluded.runtime_state,
		last_error=excluded.last_error,
		restart_count=excluded.restart_count,
		last_transition_at=excluded.last_transition_at,
		last_reconcile_at=excluded.last_reconcile_at,
		last_start_attempt_at=excluded.last_start_attempt_at,
		failure_count=excluded.failure_count,
		deleted_at=excluded.deleted_at,
		generation=excluded.generation,
		observed_generation=excluded.observed_generation,
		updated_at=excluded.updated_at`,
		ms.LocalUUID, ms.ApplicationName, ms.MicroserviceName, ms.SourceName, ms.ManifestYAML, ms.ImageName,
		ms.State, ms.ContainerID, ms.DesiredState, ms.RuntimeState, ms.LastError, ms.RestartCount, ms.LastTransitionAt,
		ms.LastReconcileAt, ms.LastStartAttemptAt, ms.FailureCount, ms.DeletedAt, ms.Generation, ms.ObservedGeneration, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert local deployed microservice: %w", err)
	}
	return nil
}

// ListLocalDeployedMicroservices returns all locally deployed microservices.
func (d *DB) ListLocalDeployedMicroservices() ([]*models.LocalDeployedMicroservice, error) {
	rows, err := d.Conn().Query(`SELECT
		local_uuid, application_name, microservice_name, source_name, manifest_yaml, image_name, state, container_id,
		desired_state, runtime_state, last_error, restart_count, last_transition_at, last_reconcile_at, last_start_attempt_at, failure_count,
		deleted_at, generation, observed_generation
		FROM local_deployed_microservices ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query local_deployed_microservices: %w", err)
	}
	defer rows.Close()

	var result []*models.LocalDeployedMicroservice
	for rows.Next() {
		item := &models.LocalDeployedMicroservice{}
		if scanErr := rows.Scan(
			&item.LocalUUID, &item.ApplicationName, &item.MicroserviceName, &item.SourceName, &item.ManifestYAML,
			&item.ImageName, &item.State, &item.ContainerID, &item.DesiredState, &item.RuntimeState, &item.LastError,
			&item.RestartCount, &item.LastTransitionAt, &item.LastReconcileAt, &item.LastStartAttemptAt, &item.FailureCount,
			&item.DeletedAt, &item.Generation, &item.ObservedGeneration,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan local_deployed_microservices row: %w", scanErr)
		}
		item.NormalizeDefaults()
		result = append(result, item)
	}
	if result == nil {
		result = make([]*models.LocalDeployedMicroservice, 0)
	}
	return result, rows.Err()
}

// GetLocalDeployedMicroservice retrieves one local deployment record by id.
func (d *DB) GetLocalDeployedMicroservice(id string) (*models.LocalDeployedMicroservice, error) {
	row := d.Conn().QueryRow(`SELECT
		local_uuid, application_name, microservice_name, source_name, manifest_yaml, image_name, state, container_id,
		desired_state, runtime_state, last_error, restart_count, last_transition_at, last_reconcile_at, last_start_attempt_at, failure_count,
		deleted_at, generation, observed_generation
		FROM local_deployed_microservices WHERE local_uuid = ?`, id)
	item := &models.LocalDeployedMicroservice{}
	if err := row.Scan(
		&item.LocalUUID, &item.ApplicationName, &item.MicroserviceName, &item.SourceName, &item.ManifestYAML,
		&item.ImageName, &item.State, &item.ContainerID, &item.DesiredState, &item.RuntimeState, &item.LastError,
		&item.RestartCount, &item.LastTransitionAt, &item.LastReconcileAt, &item.LastStartAttemptAt, &item.FailureCount,
		&item.DeletedAt, &item.Generation, &item.ObservedGeneration,
	); err != nil {
		return nil, err
	}
	item.NormalizeDefaults()
	return item, nil
}

// FindLocalDeployedMicroservicesByAppAndName finds local deployments by app/name.
func (d *DB) FindLocalDeployedMicroservicesByAppAndName(application, name string) ([]*models.LocalDeployedMicroservice, error) {
	app := strings.TrimSpace(application)
	msName := strings.TrimSpace(name)
	if app == "" || msName == "" {
		return make([]*models.LocalDeployedMicroservice, 0), nil
	}
	if d.Conn() == nil {
		return nil, fmt.Errorf("database is closed")
	}
	rows, err := d.Conn().Query(`SELECT
		local_uuid, application_name, microservice_name, source_name, manifest_yaml, image_name, state, container_id,
		desired_state, runtime_state, last_error, restart_count, last_transition_at, last_reconcile_at, last_start_attempt_at, failure_count,
		deleted_at, generation, observed_generation
		FROM local_deployed_microservices
		WHERE application_name = ? COLLATE NOCASE
		  AND microservice_name = ? COLLATE NOCASE
		ORDER BY updated_at DESC, created_at DESC`, app, msName)
	if err != nil {
		return nil, fmt.Errorf("failed to query local_deployed_microservices by app/name: %w", err)
	}
	defer rows.Close()

	items := make([]*models.LocalDeployedMicroservice, 0)
	for rows.Next() {
		item := &models.LocalDeployedMicroservice{}
		if scanErr := rows.Scan(
			&item.LocalUUID, &item.ApplicationName, &item.MicroserviceName, &item.SourceName, &item.ManifestYAML,
			&item.ImageName, &item.State, &item.ContainerID, &item.DesiredState, &item.RuntimeState, &item.LastError,
			&item.RestartCount, &item.LastTransitionAt, &item.LastReconcileAt, &item.LastStartAttemptAt, &item.FailureCount,
			&item.DeletedAt, &item.Generation, &item.ObservedGeneration,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan local_deployed_microservices by app/name row: %w", scanErr)
		}
		item.NormalizeDefaults()
		items = append(items, item)
	}
	return items, rows.Err()
}

// DeleteLocalDeployedMicroservice removes a local deployment record by id.
func (d *DB) DeleteLocalDeployedMicroservice(id string) error {
	_, err := d.Conn().Exec(`DELETE FROM local_deployed_microservices WHERE local_uuid = ?`, id)
	return err
}

// ClearLocalDeployedMicroservices removes all local deployment records.
func (d *DB) ClearLocalDeployedMicroservices() error {
	_, err := d.Conn().Exec(`DELETE FROM local_deployed_microservices`)
	return err
}
