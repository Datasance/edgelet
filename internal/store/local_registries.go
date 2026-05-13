package store

import (
	"fmt"
	"time"

	"github.com/eclipse-iofog/agent/internal/models"
)

// UpsertLocalRegistry inserts or updates one local registry row.
func (d *DB) UpsertLocalRegistry(reg *models.Registry) error {
	if reg == nil {
		return fmt.Errorf("registry is nil")
	}
	_, err := d.Conn().Exec(
		`INSERT OR REPLACE INTO local_registries (id, url, is_public, user_name, password, user_email, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		reg.ID, reg.URL, boolToInt(reg.IsPublic), reg.UserName, reg.Password, reg.UserEmail,
		time.Now().Unix(),
	)
	return err
}

// LoadLocalRegistries retrieves all local registries ordered by id.
func (d *DB) LoadLocalRegistries() ([]*models.Registry, error) {
	rows, err := d.Conn().Query(
		"SELECT id, url, is_public, user_name, password, user_email FROM local_registries ORDER BY id",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query local_registries: %w", err)
	}
	defer rows.Close()

	var result []*models.Registry
	for rows.Next() {
		reg, err := scanRegistry(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan local registry: %w", err)
		}
		result = append(result, reg)
	}
	if result == nil {
		result = make([]*models.Registry, 0)
	}
	return result, rows.Err()
}

// EnsureDefaultLocalRegistries ensures default public registries exist in local table.
func (d *DB) EnsureDefaultLocalRegistries() error {
	defaults := []*models.Registry{
		models.NewRegistry(1, "docker.io", true, "", "", ""),
		models.NewRegistry(2, "from_cache", true, "", "", ""),
	}
	for _, reg := range defaults {
		if err := d.UpsertLocalRegistry(reg); err != nil {
			return err
		}
	}
	return nil
}

// DeleteLocalRegistry deletes a local registry by ID.
func (d *DB) DeleteLocalRegistry(id int) error {
	_, err := d.Conn().Exec("DELETE FROM local_registries WHERE id = ?", id)
	return err
}

// GetLocalRegistry gets one local registry by ID.
func (d *DB) GetLocalRegistry(id int) (*models.Registry, error) {
	row := d.Conn().QueryRow(
		"SELECT id, url, is_public, user_name, password, user_email FROM local_registries WHERE id = ?",
		id,
	)
	reg := &models.Registry{}
	var isPublic int
	if err := row.Scan(&reg.ID, &reg.URL, &isPublic, &reg.UserName, &reg.Password, &reg.UserEmail); err != nil {
		return nil, err
	}
	reg.IsPublic = intToBool(isPublic)
	return reg, nil
}
