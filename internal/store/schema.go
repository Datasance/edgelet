package store

import (
	"embed"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const storageModuleName = "SQLite Store"

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrate bootstraps the schema_versions table, then applies all .sql files
// whose version number exceeds the currently stored maximum version.
func (d *DB) migrate() error {
	// Bootstrap: ensure schema_versions exists before anything else
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_versions (
		version     INTEGER PRIMARY KEY,
		description TEXT    NOT NULL,
		applied_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`); err != nil {
		return fmt.Errorf("failed to bootstrap schema_versions: %w", err)
	}

	// Determine current schema version
	var currentVersion int
	if err := d.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_versions").Scan(&currentVersion); err != nil {
		return fmt.Errorf("failed to read current schema version: %w", err)
	}

	// Enumerate migration files sorted by name (001_..., 002_..., etc.)
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
			logging.LogWarn(storageModuleName, fmt.Sprintf("Skipping migration file with unparseable version: %s", entry.Name()))
			continue
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
		logging.LogInfo(storageModuleName, fmt.Sprintf("Migration v%d applied successfully", version))
	}

	return nil
}

// runMigration executes all statements in a single .sql file inside a transaction.
// Statements that fail with "already exists" or "UNIQUE constraint" errors are
// logged as warnings and skipped rather than aborting the migration.
func (d *DB) runMigration(version int, description, sqlContent string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, stmt := range splitStatements(sqlContent) {
		if _, err := tx.Exec(stmt); err != nil {
			if isAlreadyExistsOrDuplicate(err) {
				logging.LogWarn(storageModuleName, fmt.Sprintf("Migration v%d statement skipped (already applied): %v", version, err))
				continue
			}
			return fmt.Errorf("statement failed: %w\nSQL: %s", err, stmt)
		}
	}

	// Record this migration as applied
	if _, err := tx.Exec(
		"INSERT OR IGNORE INTO schema_versions (version, description) VALUES (?, ?)",
		version, description,
	); err != nil {
		return fmt.Errorf("failed to record schema version: %w", err)
	}

	return tx.Commit()
}

// parseMigrationVersion extracts the leading integer from a filename like "001_initial_schema.sql"
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

// splitStatements splits a SQL string on ";" and returns non-empty trimmed statements
func splitStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if s := strings.TrimSpace(part); s != "" {
			result = append(result, s)
		}
	}
	return result
}

// isAlreadyExistsOrDuplicate returns true for errors indicating a statement was
// already applied: table exists, UNIQUE constraint, or duplicate entry.
func isAlreadyExistsOrDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "duplicate")
}
