package handlers

import (
	"testing"

	"github.com/eclipse-iofog/agent/internal/store"
)

// ensureStoreDBOpen keeps handler tests from failing when another test closed the store singleton.
func ensureStoreDBOpen(t *testing.T) {
	t.Helper()
	db := store.GetInstance()
	if conn := db.Conn(); conn != nil {
		if err := conn.Ping(); err == nil {
			return
		}
		_ = db.Close()
	}
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open store db: %v", err)
	}
}
