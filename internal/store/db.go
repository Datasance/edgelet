package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const dbFileName = "edgelet.db"

var (
	instance *DB
	once     sync.Once
)

// DB wraps the SQLite database connection
type DB struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// GetInstance returns the singleton DB instance
func GetInstance() *DB {
	once.Do(func() {
		instance = &DB{}
	})
	return instance
}

// Open opens (or creates) the SQLite database at the given directory and runs migrations
func (d *DB) Open(dir string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create db directory: %w", err)
	}

	d.path = filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", d.path+"?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=ON")
	if err != nil {
		return fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Single writer to avoid lock contention
	db.SetMaxOpenConns(1)

	d.db = db
	return d.migrate()
}

// Close closes the database connection
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Conn returns the underlying *sql.DB
func (d *DB) Conn() *sql.DB {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.db
}
