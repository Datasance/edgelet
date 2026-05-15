//go:build (linux && amd64) || (linux && arm64) || (linux && arm) || (linux && riscv64)

package embedded

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractBinaryCreatesFileWhenMissing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "binary")
	payload := []byte("abc123")

	if err := extractBinary(payload, dest); err != nil {
		t.Fatalf("extractBinary failed: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("binary content mismatch: got=%q want=%q", got, payload)
	}
}

func TestExtractBinarySkipsRewriteWhenContentUnchanged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "binary")
	payload := []byte("same-content")

	if err := extractBinary(payload, dest); err != nil {
		t.Fatalf("initial extractBinary failed: %v", err)
	}

	infoBefore, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat before rewrite: %v", err)
	}

	// Ensure timestamp resolution boundary is crossed so mtime changes are observable.
	time.Sleep(1100 * time.Millisecond)

	if err := extractBinary(payload, dest); err != nil {
		t.Fatalf("second extractBinary failed: %v", err)
	}

	infoAfter, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat after rewrite: %v", err)
	}

	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatalf("expected unchanged file to keep mtime, got before=%s after=%s", infoBefore.ModTime(), infoAfter.ModTime())
	}
}

func TestExtractBinaryReplacesWhenContentChanged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "binary")

	first := []byte("first")
	second := []byte("second")

	if err := extractBinary(first, dest); err != nil {
		t.Fatalf("initial extractBinary failed: %v", err)
	}

	infoBefore, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat before replace: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	if err := extractBinary(second, dest); err != nil {
		t.Fatalf("replace extractBinary failed: %v", err)
	}

	infoAfter, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat after replace: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}

	if !bytes.Equal(got, second) {
		t.Fatalf("binary content mismatch after replace: got=%q want=%q", got, second)
	}
	if !infoAfter.ModTime().After(infoBefore.ModTime()) {
		t.Fatalf("expected replaced file to have newer mtime, before=%s after=%s", infoBefore.ModTime(), infoAfter.ModTime())
	}
}
