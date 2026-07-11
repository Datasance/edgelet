//go:build linux

package edgelet

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"testing"
)

func minimalImageTar(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Mode: 0o644,
		Size: 2,
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte("{}")); err != nil {
		t.Fatalf("write tar payload: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	return buf.Bytes()
}

func TestDecompressImageArchive_UncompressedTar(t *testing.T) {
	raw := minimalImageTar(t)
	rc, err := decompressImageArchive(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decompressImageArchive: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read decompressed archive: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("expected uncompressed tar to pass through unchanged")
	}
}

func TestDecompressImageArchive_GzipTar(t *testing.T) {
	raw := minimalImageTar(t)
	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	if _, err := gw.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	rc, err := decompressImageArchive(bytes.NewReader(gz.Bytes()))
	if err != nil {
		t.Fatalf("decompressImageArchive: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read decompressed archive: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("expected gzip tar to decompress to original tar bytes")
	}
}
