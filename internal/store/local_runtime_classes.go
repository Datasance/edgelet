package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/models"
)

// UpsertLocalRuntimeClass inserts or updates one RuntimeClass row.
func (d *DB) UpsertLocalRuntimeClass(rc *models.LocalRuntimeClass) error {
	if rc == nil {
		return fmt.Errorf("runtime class is nil")
	}
	rc.Normalize()
	if rc.Name == "" {
		return fmt.Errorf("runtime class name is required")
	}
	if rc.Handler == "" {
		return fmt.Errorf("runtime class handler is required")
	}

	_, err := d.Conn().Exec(
		`INSERT INTO local_runtime_classes (name, handler, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		 	handler=excluded.handler,
		 	updated_at=excluded.updated_at`,
		rc.Name, rc.Handler, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert local runtime class: %w", err)
	}
	return nil
}

// ListLocalRuntimeClasses returns all RuntimeClass rows ordered by name.
func (d *DB) ListLocalRuntimeClasses() ([]*models.LocalRuntimeClass, error) {
	rows, err := d.Conn().Query(
		`SELECT name, handler, created_at, updated_at
		 FROM local_runtime_classes ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query local_runtime_classes: %w", err)
	}
	defer rows.Close()

	items := make([]*models.LocalRuntimeClass, 0)
	for rows.Next() {
		rc := &models.LocalRuntimeClass{}
		if scanErr := rows.Scan(&rc.Name, &rc.Handler, &rc.CreatedAt, &rc.UpdatedAt); scanErr != nil {
			return nil, fmt.Errorf("failed to scan local runtime class: %w", scanErr)
		}
		rc.Normalize()
		items = append(items, rc)
	}
	return items, rows.Err()
}

// GetLocalRuntimeClass returns one RuntimeClass by name.
func (d *DB) GetLocalRuntimeClass(name string) (*models.LocalRuntimeClass, error) {
	normalizedName := strings.TrimSpace(strings.ToLower(name))
	row := d.Conn().QueryRow(
		`SELECT name, handler, created_at, updated_at
		 FROM local_runtime_classes WHERE name = ?`,
		normalizedName,
	)
	rc := &models.LocalRuntimeClass{}
	if err := row.Scan(&rc.Name, &rc.Handler, &rc.CreatedAt, &rc.UpdatedAt); err != nil {
		return nil, err
	}
	rc.Normalize()
	return rc, nil
}

// DeleteLocalRuntimeClass deletes one RuntimeClass by name.
func (d *DB) DeleteLocalRuntimeClass(name string) error {
	normalizedName := strings.TrimSpace(strings.ToLower(name))
	_, err := d.Conn().Exec(`DELETE FROM local_runtime_classes WHERE name = ?`, normalizedName)
	return err
}

// ClearLocalRuntimeClasses removes all RuntimeClass rows.
func (d *DB) ClearLocalRuntimeClasses() error {
	_, err := d.Conn().Exec(`DELETE FROM local_runtime_classes`)
	return err
}
