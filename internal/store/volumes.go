package store

import (
	"encoding/json"
	"fmt"
	"time"
)

// VolumeMountRecord represents a single volume mount index entry.
// UUID, Name, Version, Kind, Checksum are real typed columns.
// Microservices and Data are JSON-serialized TEXT columns.
type VolumeMountRecord struct {
	UUID          string
	Name          string
	Version       float64
	Kind          string // "SECRET" or "CONFIGMAP"
	Checksum      string
	Microservices []string               // stored as JSON TEXT array
	Data          map[string]interface{} // stored as JSON TEXT (key->base64 pairs)
	UpdatedAt     int64
}

// UpsertVolumeMount inserts or replaces a volume mount record.
func (d *DB) UpsertVolumeMount(rec VolumeMountRecord) error {
	microservicesJSON, err := json.Marshal(rec.Microservices)
	if err != nil {
		return fmt.Errorf("failed to marshal microservices list: %w", err)
	}
	dataJSON, err := json.Marshal(rec.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal volume mount data: %w", err)
	}

	_, err = d.Conn().Exec(
		`INSERT OR REPLACE INTO volume_mounts
		 (uuid, name, version, kind, checksum, microservices, data, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.UUID, rec.Name, rec.Version, rec.Kind, rec.Checksum,
		string(microservicesJSON), string(dataJSON),
		time.Now().Unix(),
	)
	return err
}

// DeleteVolumeMount removes a volume mount record by UUID.
func (d *DB) DeleteVolumeMount(uuid string) error {
	_, err := d.Conn().Exec("DELETE FROM volume_mounts WHERE uuid = ?", uuid)
	return err
}

// LoadAllVolumeMounts retrieves all volume mount records.
func (d *DB) LoadAllVolumeMounts() ([]VolumeMountRecord, error) {
	rows, err := d.Conn().Query(
		"SELECT uuid, name, version, kind, checksum, microservices, data, updated_at FROM volume_mounts",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query volume mounts: %w", err)
	}
	defer rows.Close()

	var result []VolumeMountRecord
	for rows.Next() {
		var rec VolumeMountRecord
		var microservicesJSON, dataJSON string
		if err := rows.Scan(
			&rec.UUID, &rec.Name, &rec.Version, &rec.Kind, &rec.Checksum,
			&microservicesJSON, &dataJSON, &rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan volume mount: %w", err)
		}
		json.Unmarshal([]byte(microservicesJSON), &rec.Microservices) // #nosec G104 -- data written by this process; parse failure yields empty/zero value
		json.Unmarshal([]byte(dataJSON), &rec.Data)                   // #nosec G104 -- data written by this process; parse failure yields empty/zero value
		if rec.Microservices == nil {
			rec.Microservices = make([]string, 0)
		}
		if rec.Data == nil {
			rec.Data = make(map[string]interface{})
		}
		result = append(result, rec)
	}
	if result == nil {
		result = make([]VolumeMountRecord, 0)
	}
	return result, rows.Err()
}

// GetVolumeMountByUUID retrieves a single volume mount record by UUID.
func (d *DB) GetVolumeMountByUUID(uuid string) (*VolumeMountRecord, error) {
	row := d.Conn().QueryRow(
		"SELECT uuid, name, version, kind, checksum, microservices, data, updated_at FROM volume_mounts WHERE uuid = ?",
		uuid,
	)
	var rec VolumeMountRecord
	var microservicesJSON, dataJSON string
	if err := row.Scan(
		&rec.UUID, &rec.Name, &rec.Version, &rec.Kind, &rec.Checksum,
		&microservicesJSON, &dataJSON, &rec.UpdatedAt,
	); err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(microservicesJSON), &rec.Microservices) // #nosec G104 -- data written by this process; parse failure yields empty/zero value
	json.Unmarshal([]byte(dataJSON), &rec.Data)                   // #nosec G104 -- data written by this process; parse failure yields empty/zero value
	if rec.Microservices == nil {
		rec.Microservices = make([]string, 0)
	}
	if rec.Data == nil {
		rec.Data = make(map[string]interface{})
	}
	return &rec, nil
}

// ClearAllVolumeMounts removes all volume mount rows (used on deprovision).
func (d *DB) ClearAllVolumeMounts() error {
	_, err := d.Conn().Exec("DELETE FROM volume_mounts")
	return err
}

// ReplaceAllVolumeMounts atomically replaces all volume mount rows.
// Used by the volume mount manager's full-save on every write.
func (d *DB) ReplaceAllVolumeMounts(records []VolumeMountRecord) error {
	tx, err := d.Conn().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // #nosec G104 -- data written by this process; parse failure yields empty/zero value

	if _, err := tx.Exec("DELETE FROM volume_mounts"); err != nil {
		return fmt.Errorf("failed to clear volume_mounts: %w", err)
	}

	now := time.Now().Unix()
	for _, rec := range records {
		msJSON, _ := json.Marshal(rec.Microservices)
		dataJSON, _ := json.Marshal(rec.Data)
		if _, err := tx.Exec(
			`INSERT INTO volume_mounts (uuid, name, version, kind, checksum, microservices, data, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			rec.UUID, rec.Name, rec.Version, rec.Kind, rec.Checksum,
			string(msJSON), string(dataJSON), now,
		); err != nil {
			return fmt.Errorf("failed to insert volume mount %s: %w", rec.UUID, err)
		}
	}

	return tx.Commit()
}
