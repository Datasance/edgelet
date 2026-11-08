// Package main writes the zstd bundle the thin binary embeds.
// The bundle contains the fat runtime only.
package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eclipse-iofog/edgelet-kube/internal/bundle"
	"github.com/klauspost/compress/zstd"
)

func main() {
	bin := flag.String("bin", "", "fat runtime ELF")
	out := flag.String("out", "", "directory for the zstd bundle")
	flag.Parse()
	if err := pack(*bin, *out); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "pack: %v\n", err)
		os.Exit(1)
	}
}

func pack(bin, out string) error {
	if bin == "" || out == "" {
		return fmt.Errorf("bin and out are required")
	}
	body, err := os.ReadFile(bin)
	if err != nil {
		return fmt.Errorf("read runtime: %w", err)
	}
	if len(body) == 0 {
		return fmt.Errorf("runtime is empty")
	}
	packed, err := bundleBytes(body)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(packed)
	name := hex.EncodeToString(sum[:]) + ".tar.zst"
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("mkdir bundle dir: %w", err)
	}
	old, err := filepath.Glob(filepath.Join(out, "*.tar.zst"))
	if err != nil {
		return fmt.Errorf("list bundles: %w", err)
	}
	for _, path := range old {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	dest := filepath.Join(out, name)
	if err := os.WriteFile(dest, packed, 0o644); err != nil {
		return fmt.Errorf("write bundle: %w", err)
	}
	fmt.Println(dest)
	return nil
}

func bundleBytes(body []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		return nil, fmt.Errorf("zstd writer: %w", err)
	}
	tw := tar.NewWriter(zw)
	hdr := &tar.Header{
		Name: bundle.FatPath,
		Mode: 0o755,
		Size: int64(len(body)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("tar header: %w", err)
	}
	if _, err := tw.Write(body); err != nil {
		return nil, fmt.Errorf("tar write: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("tar close: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("zstd close: %w", err)
	}
	return buf.Bytes(), nil
}
