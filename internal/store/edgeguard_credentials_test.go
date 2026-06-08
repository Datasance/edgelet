package store

import (
	"testing"
)

func TestEdgeGuardSignatureCRUD(t *testing.T) {
	tempDir := t.TempDir()
	db := &DB{}
	if err := db.Open(tempDir); err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}( //nolint:errcheck
	)

	if err := db.UpsertEdgeGuardSignature("sig-1"); err != nil {
		t.Fatalf("upsert signature: %v", err)
	}

	got, found, err := db.GetEdgeGuardSignature()
	if err != nil {
		t.Fatalf("get signature: %v", err)
	}
	if !found || got != "sig-1" {
		t.Fatalf("expected signature sig-1 found=true, got %q found=%v", got, found)
	}

	if err := db.UpsertEdgeGuardSignature("sig-2"); err != nil {
		t.Fatalf("upsert signature second value: %v", err)
	}

	got, found, err = db.GetEdgeGuardSignature()
	if err != nil {
		t.Fatalf("get signature after update: %v", err)
	}
	if !found || got != "sig-2" {
		t.Fatalf("expected signature sig-2 found=true, got %q found=%v", got, found)
	}

	if err := db.DeleteEdgeGuardSignature(); err != nil {
		t.Fatalf("delete signature: %v", err)
	}

	_, found, err = db.GetEdgeGuardSignature()
	if err != nil {
		t.Fatalf("get signature after delete: %v", err)
	}
	if found {
		t.Fatal("expected no signature after delete")
	}
}

func TestAgentPrivateKeyCRUD(t *testing.T) {
	tempDir := t.TempDir()
	db := &DB{}
	if err := db.Open(tempDir); err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}( //nolint:errcheck
	)

	if err := db.UpsertAgentPrivateKey("key-a"); err != nil {
		t.Fatalf("upsert private key: %v", err)
	}

	got, found, err := db.GetAgentPrivateKey()
	if err != nil {
		t.Fatalf("get private key: %v", err)
	}
	if !found || got != "key-a" {
		t.Fatalf("expected private key key-a found=true, got %q found=%v", got, found)
	}

	if err := db.UpsertAgentPrivateKey("key-b"); err != nil {
		t.Fatalf("upsert private key second value: %v", err)
	}

	got, found, err = db.GetAgentPrivateKey()
	if err != nil {
		t.Fatalf("get private key after update: %v", err)
	}
	if !found || got != "key-b" {
		t.Fatalf("expected private key key-b found=true, got %q found=%v", got, found)
	}

	if err := db.DeleteAgentPrivateKey(); err != nil {
		t.Fatalf("delete private key: %v", err)
	}

	_, found, err = db.GetAgentPrivateKey()
	if err != nil {
		t.Fatalf("get private key after delete: %v", err)
	}
	if found {
		t.Fatal("expected no private key after delete")
	}
}
