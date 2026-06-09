package dataverify

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzFileMapFields(f *testing.F) {
	f.Add([]byte("abc123  relative/path\n"))
	f.Add([]byte("link  target\n"))
	f.Add([]byte("malformed\n"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		manifest := filepath.Join(dir, "manifest")
		if err := os.WriteFile(manifest, data, 0644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		for _, keyVal := range [][2]int{{1, 0}, {0, 1}} {
			_, _ = fileMapFields(manifest, keyVal[0], keyVal[1])
		}
	})
}
