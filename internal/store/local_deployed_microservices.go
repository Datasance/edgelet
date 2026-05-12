package store

import (
	"fmt"
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

	_, err := d.Conn().Exec(`INSERT OR REPLACE INTO local_deployed_microservices (
		local_uuid, application_name, microservice_name, source_name, manifest_yaml, image_name,
		state, container_id, created_at, updated_at
	) VALUES (
		?,?,?,?,?,?,?,?,
		COALESCE((SELECT created_at FROM local_deployed_microservices WHERE local_uuid = ?), strftime('%s','now')),
		?
	)`,
		ms.LocalUUID, ms.ApplicationName, ms.MicroserviceName, ms.SourceName, ms.ManifestYAML, ms.ImageName,
		ms.State, ms.ContainerID, ms.LocalUUID, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert local deployed microservice: %w", err)
	}
	return nil
}

// ListLocalDeployedMicroservices returns all locally deployed microservices.
func (d *DB) ListLocalDeployedMicroservices() ([]*models.LocalDeployedMicroservice, error) {
	rows, err := d.Conn().Query(`SELECT
		local_uuid, application_name, microservice_name, source_name, manifest_yaml, image_name, state, container_id
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
			&item.ImageName, &item.State, &item.ContainerID,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan local_deployed_microservices row: %w", scanErr)
		}
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
		local_uuid, application_name, microservice_name, source_name, manifest_yaml, image_name, state, container_id
		FROM local_deployed_microservices WHERE local_uuid = ?`, id)
	item := &models.LocalDeployedMicroservice{}
	if err := row.Scan(
		&item.LocalUUID, &item.ApplicationName, &item.MicroserviceName, &item.SourceName, &item.ManifestYAML,
		&item.ImageName, &item.State, &item.ContainerID,
	); err != nil {
		return nil, err
	}
	return item, nil
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
