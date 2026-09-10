package store

import (
	"fmt"
	"strings"
)

// ListReferencedModelNames returns names that must be kept during dangling prune:
// deployed local model rows plus any explicit model_refs rows (workload binds later).
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

// DeleteModelRefs removes explicit references for one model name.
func (d *DB) DeleteModelRefs(name string) error {
	_, err := d.Conn().Exec("DELETE FROM model_refs WHERE model_name = ?", strings.TrimSpace(name))
	return err
}
