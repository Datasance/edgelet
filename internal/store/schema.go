package store

import (
	"embed"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/datasance/edgelet/internal/utils/logging"
)

const storageModuleName = "SQLite Store"

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrate bootstraps the schema_versions table, then applies all .sql files
// whose version number exceeds the currently stored maximum version.
func (d *DB) migrate() error {
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_versions (
		version     INTEGER PRIMARY KEY,
		description TEXT    NOT NULL,
		applied_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`); err != nil {
		return fmt.Errorf("failed to bootstrap schema_versions: %w", err)
	}

	var currentVersion int
	if err := d.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_versions").Scan(&currentVersion); err != nil {
		return fmt.Errorf("failed to read current schema version: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migration directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := parseMigrationVersion(entry.Name())
		if err != nil {
			return fmt.Errorf("unparseable migration filename %s: %w", entry.Name(), err)
		}

		if version <= currentVersion {
			logging.LogDebug(storageModuleName, fmt.Sprintf("Migration v%d already applied, skipping", version))
			continue
		}

		sqlBytes, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}

		logging.LogInfo(storageModuleName, fmt.Sprintf("Applying migration v%d: %s", version, entry.Name()))
		if err := d.runMigration(version, entry.Name(), string(sqlBytes)); err != nil {
			return fmt.Errorf("migration v%d failed: %w", version, err)
		}
		if err := d.vacuumAfterSchemaBump(version); err != nil {
			return fmt.Errorf("post-migration maintenance after v%d: %w", version, err)
		}
		logging.LogInfo(storageModuleName, fmt.Sprintf("Migration v%d applied successfully", version))
	}

	return nil
}

// runMigration executes all statements in a single .sql file inside a transaction.
// v1 SQL is semicolon-free: one statement per non-empty, non-comment line.
func (d *DB) runMigration(version int, description, sqlContent string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, stmt := range splitStatements(sqlContent) {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("statement failed: %w\nSQL: %s", err, stmt)
		}
	}

	if _, err := tx.Exec(
		"INSERT INTO schema_versions (version, description) VALUES (?, ?)",
		version, description,
	); err != nil {
		return fmt.Errorf("failed to record schema version: %w", err)
	}

	return tx.Commit()
}

// vacuumAfterSchemaBump runs an optional one-time PRAGMA VACUUM after a newly applied
// schema migration. Schema v1 skips this (no VACUUM on every boot). Enable for v2+
// when adding 002_*.sql so file layout is compacted once per upgrade, not each Open.
func (d *DB) vacuumAfterSchemaBump(appliedVersion int) error {
	if appliedVersion < 2 {
		return nil
	}
	logging.LogInfo(storageModuleName, fmt.Sprintf("Running post-migration VACUUM after schema v%d", appliedVersion))
	if _, err := d.db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("VACUUM after schema v%d: %w", appliedVersion, err)
	}
	return nil
}

func parseMigrationVersion(name string) (int, error) {
	base := filepath.Base(name)
	parts := strings.SplitN(base, "_", 2)
	if len(parts) == 0 {
		return 0, fmt.Errorf("no underscore in migration filename: %s", name)
	}
	v, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("non-numeric version prefix in %s: %w", name, err)
	}
	return v, nil
}

// splitStatements returns one SQL statement per line block.
// Blocks are separated by blank lines; consecutive non-comment lines form one statement.
// Migration files must not use semicolons as statement separators.
func splitStatements(sql string) []string {
	var result []string
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		result = append(result, strings.TrimSpace(buf.String()))
		buf.Reset()
	}

	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			flush()
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(trimmed)
	}
	flush()
	return result
}
