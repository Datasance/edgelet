package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

const controllerModelSelectColumns = `id, name, repo, revision, registry_id, files_json, format`

// SaveControllerModels replaces all controller model rows in a single transaction.
func (d *DB) SaveControllerModels(items []*models.ControllerModel) error {
	tx, err := d.Conn().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec("DELETE FROM controller_models"); err != nil {
		return fmt.Errorf("failed to clear controller_models: %w", err)
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		item.NormalizeDefaults()
		if item.ID <= 0 {
			return errors.New("controller model id is required")
		}
		if _, err := tx.Exec(
			`INSERT INTO controller_models (id, name, repo, revision, registry_id, files_json, format, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.Name, item.Repo, item.Revision, item.RegistryID, item.FilesJSON, item.Format,
			time.Now().Unix(),
		); err != nil {
			return fmt.Errorf("failed to insert controller model %d: %w", item.ID, err)
		}
	}
	return tx.Commit()
}

// LoadControllerModels retrieves all controller model rows ordered by id.
func (d *DB) LoadControllerModels() ([]*models.ControllerModel, error) {
	rows, err := d.Conn().Query(
		"SELECT " + controllerModelSelectColumns + " FROM controller_models ORDER BY id",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query controller_models: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	items := make([]*models.ControllerModel, 0)
	for rows.Next() {
		item, scanErr := scanControllerModel(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan controller model: %w", scanErr)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ClearControllerModels removes all controller model rows.
func (d *DB) ClearControllerModels() error {
	_, err := d.Conn().Exec("DELETE FROM controller_models")
	return err
}

func scanControllerModel(s interface{ Scan(dest ...any) error }) (*models.ControllerModel, error) {
	item := &models.ControllerModel{}
	if err := s.Scan(
		&item.ID, &item.Name, &item.Repo, &item.Revision, &item.RegistryID, &item.FilesJSON, &item.Format,
	); err != nil {
		return nil, err
	}
	item.NormalizeDefaults()
	return item, nil
}
