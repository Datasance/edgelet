package store

import (
	"errors"
	"fmt"
	"strings"
)

// ModelRefKindWorkload is the model_refs.kind value for a microservice catalog bind.
const ModelRefKindWorkload = "workload"

// ListReferencedModelNames returns names that must be kept during dangling prune:
// deployed local model rows plus any explicit model_refs rows (workload binds).
func (d *DB) ListReferencedModelNames() ([]string, error) {
	rows, err := d.Conn().Query(`
		SELECT name FROM local_models
		UNION
		SELECT model_name FROM model_refs`)
	if err != nil {
		return nil, fmt.Errorf("failed to list referenced model names: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan referenced model name: %w", err)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// CountModelRefs returns how many explicit refs exist for one model name.
func (d *DB) CountModelRefs(name string) (int, error) {
	var n int
	err := d.Conn().QueryRow(
		"SELECT COUNT(*) FROM model_refs WHERE model_name = ?",
		strings.TrimSpace(name),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count model refs: %w", err)
	}
	return n, nil
}

// ReplaceWorkloadModelRefs replaces catalog binds for one microservice UUID.
func (d *DB) ReplaceWorkloadModelRefs(msUUID string, names []string) error {
	msUUID = strings.TrimSpace(msUUID)
	if msUUID == "" {
		return errors.New("microservice uuid is required")
	}
	tx, err := d.Conn().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		"DELETE FROM model_refs WHERE kind = ? AND ref_id = ?",
		ModelRefKindWorkload, msUUID,
	); err != nil {
		return fmt.Errorf("failed to clear workload model refs: %w", err)
	}

	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		if _, err := tx.Exec(
			`INSERT INTO model_refs (model_name, kind, ref_id) VALUES (?, ?, ?)`,
			name, ModelRefKindWorkload, msUUID,
		); err != nil {
			return fmt.Errorf("failed to insert model ref %s: %w", name, err)
		}
	}
	return tx.Commit()
}

// DeleteWorkloadModelRefs removes catalog binds for one microservice UUID.
func (d *DB) DeleteWorkloadModelRefs(msUUID string) error {
	_, err := d.Conn().Exec(
		"DELETE FROM model_refs WHERE kind = ? AND ref_id = ?",
		ModelRefKindWorkload, strings.TrimSpace(msUUID),
	)
	if err != nil {
		return fmt.Errorf("failed to delete workload model refs: %w", err)
	}
	return nil
}

// DeleteModelRefs removes explicit references for one model name.
func (d *DB) DeleteModelRefs(name string) error {
	_, err := d.Conn().Exec("DELETE FROM model_refs WHERE model_name = ?", strings.TrimSpace(name))
	return err
}
