package dataverify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsELF(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	elfPath := filepath.Join(tmp, "edgelet")
	if err := os.WriteFile(elfPath, []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write elf stub: %v", err)
	}
	ok, err := IsELF(elfPath)
	if err != nil || !ok {
		t.Fatalf("expected ELF, ok=%v err=%v", ok, err)
	}

	textPath := filepath.Join(tmp, "not-elf")
	if err := os.WriteFile(textPath, []byte("not elf"), 0644); err != nil {
		t.Fatalf("write text stub: %v", err)
	}
	ok, err = IsELF(textPath)
	if err != nil || ok {
		t.Fatalf("expected non-ELF, ok=%v err=%v", ok, err)
	}
}

func TestVerifyFatRuntime(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "edgelet")
	if err := os.WriteFile(path, []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write elf stub: %v", err)
	}
	if err := VerifyFatRuntime(path); err != nil {
		t.Fatalf("VerifyFatRuntime: %v", err)
	}
}

func TestVerifyFatRuntimeRejectsNonELF(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "edgelet")
	if err := os.WriteFile(path, []byte("not-an-elf"), 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	if err := VerifyFatRuntime(path); err == nil {
		t.Fatal("expected non-ELF rejection")
	}
}
