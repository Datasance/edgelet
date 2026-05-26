package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/datasance/edgelet/internal/models"
)

// SaveRegistries replaces all registry rows in a single transaction.
func (d *DB) SaveRegistries(registries []*models.Registry) error {
	tx, err := d.Conn().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec("DELETE FROM registries"); err != nil {
		return fmt.Errorf("failed to clear registries: %w", err)
	}

	for _, reg := range registries {
		if err := insertRegistry(tx, reg); err != nil {
			return fmt.Errorf("failed to insert registry %d: %w", reg.ID, err)
		}
	}

	return tx.Commit()
}

// LoadRegistries retrieves all registries ordered by id.
func (d *DB) LoadRegistries() ([]*models.Registry, error) {
	rows, err := d.Conn().Query(
		"SELECT id, url, is_public, user_name, password, user_email FROM registries ORDER BY id",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query registries: %w", err)
	}
	defer rows.Close()

	var result []*models.Registry
	for rows.Next() {
		reg, err := scanRegistry(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan registry: %w", err)
		}
		result = append(result, reg)
	}
	if result == nil {
		result = make([]*models.Registry, 0)
	}
	return result, rows.Err()
}

// ClearRegistries removes all registry rows (used on deprovision).
func (d *DB) ClearRegistries() error {
	_, err := d.Conn().Exec("DELETE FROM registries")
	return err
}

func insertRegistry(tx *sql.Tx, reg *models.Registry) error {
	_, err := tx.Exec(
		`INSERT OR REPLACE INTO registries (id, url, is_public, user_name, password, user_email, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		reg.ID, reg.URL, boolToInt(reg.IsPublic), reg.UserName, reg.Password, reg.UserEmail,
		time.Now().Unix(),
	)
	return err
}

func scanRegistry(rows *sql.Rows) (*models.Registry, error) {
	reg := &models.Registry{}
	var isPublic int
	if err := rows.Scan(&reg.ID, &reg.URL, &isPublic, &reg.UserName, &reg.Password, &reg.UserEmail); err != nil {
		return nil, err
	}
	reg.IsPublic = intToBool(isPublic)
	return reg, nil
}

// UpsertRegistry inserts or updates one registry row.
func (d *DB) UpsertRegistry(reg *models.Registry) error {
	if reg == nil {
		return fmt.Errorf("registry is nil")
	}
	_, err := d.Conn().Exec(
		`INSERT OR REPLACE INTO registries (id, url, is_public, user_name, password, user_email, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		reg.ID, reg.URL, boolToInt(reg.IsPublic), reg.UserName, reg.Password, reg.UserEmail,
		time.Now().Unix(),
	)
	return err
}

// EnsureDefaultRegistries ensures docker.io and from_cache defaults exist.
func (d *DB) EnsureDefaultRegistries() error {
	defaults := []*models.Registry{
		models.NewRegistry(1, "docker.io", true, "", "", ""),
		models.NewRegistry(2, "from_cache", true, "", "", ""),
	}
	for _, reg := range defaults {
		if err := d.UpsertRegistry(reg); err != nil {
			return err
		}
	}
	return nil
}
