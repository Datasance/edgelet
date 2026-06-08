package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDB_Open_IntegrityCheckFailsOnCorruptDatabase(t *testing.T) {
	dir := t.TempDir()
	db := &DB{}
	if err := db.Open(dir); err != nil {
		t.Fatalf("initial open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	path := filepath.Join(dir, dbFileName)
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("open db file: %v", err)
	}
	if _, err := f.WriteAt(make([]byte, 16), 0); err != nil {
		t.Fatalf("corrupt db header: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close db file: %v", err)
	}

	broken := &DB{}
	if openErr := broken.Open(dir); openErr == nil {
		_ = broken.Close()
		t.Fatal("expected Open to fail hard on corrupt database")
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open corrupt file: %v", err)
	}
	defer func() {
		_ = raw.Close()
	}( //nolint:errcheck
	)

	damaged := &DB{db: raw, path: path}
	icErr := damaged.checkIntegrity()
	if icErr == nil {
		t.Fatal("expected checkIntegrity to fail on corrupt file")
	}
	if !strings.Contains(icErr.Error(), "integrity_check") {
		t.Fatalf("expected integrity_check error, got: %v", icErr)
	}
}

func TestDB_Close_WALCheckpointTruncate(t *testing.T) {
	dir := t.TempDir()
	db := &DB{}
	if err := db.Open(dir); err != nil {
		t.Fatalf("open: %v", err)
	}

	var journalMode string
	if err := db.Conn().QueryRow(`PRAGMA journal_mode=WAL`).Scan(&journalMode); err != nil {
		t.Fatalf("enable WAL: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected WAL journal, got %q", journalMode)
	}

	if _, err := db.Conn().Exec(
		`INSERT INTO local_runtime_classes (name, handler, created_at, updated_at) VALUES ('cp-test', 'test', 1, 1)`,
	); err != nil {
		t.Fatalf("insert to generate WAL traffic: %v", err)
	}

	var busyBefore, logBefore, ckptBefore int
	if err := db.Conn().QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&busyBefore, &logBefore, &ckptBefore); err != nil {
		t.Fatalf("wal_checkpoint before close: %v", err)
	}
	if logBefore <= 0 {
		t.Fatalf("expected WAL frames before close after write, log=%d ckpt=%d", logBefore, ckptBefore)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopen := &DB{}
	if err := reopen.Open(dir); err != nil {
		t.Fatalf("reopen after checkpoint close: %v", err)
	}
	defer func() {
		_ = reopen.Close()
	}( //nolint:errcheck
	)

	var busyAfter, logAfter, ckptAfter int
	if err := reopen.Conn().QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&busyAfter, &logAfter, &ckptAfter); err != nil {
		t.Fatalf("wal_checkpoint after reopen: %v", err)
	}
	if logAfter > 0 {
		t.Fatalf("expected no WAL frames after TRUNCATE close, log=%d ckpt=%d", logAfter, ckptAfter)
	}

	walPath := filepath.Join(dir, dbFileName+"-wal")
	if st, err := os.Stat(walPath); err == nil && st.Size() > 0 {
		t.Fatalf("expected WAL sidecar truncated after close, size=%d", st.Size())
	}
}

func TestDB_Open_IntegrityCheckPassesOnFreshDatabase(t *testing.T) {
	dir := t.TempDir()
	db := &DB{}
	if err := db.Open(dir); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var result string
	if err := db.Conn().QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected integrity_check ok, got %q", result)
	}
}
