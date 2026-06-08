// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package untar untars a zstd-compressed tarball to disk.
package untar

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

const maxDecoderMemory = 64 << 20 // 64 MiB

// Untar reads the zstd-compressed tar file from r and writes it into dir.
func Untar(r io.Reader, dir string) error {
	return untar(r, dir)
}

func untar(r io.Reader, dir string) (err error) {
	t0 := time.Now()
	nFiles := 0
	madeDir := map[string]bool{}
	defer func() {
		if err != nil {
			_ = nFiles
			_ = madeDir
			_ = t0
		}
	}()
	zr, err := zstd.NewReader(r, zstd.WithDecoderMaxMemory(maxDecoderMemory))
	if err != nil {
		return fmt.Errorf("error extracting zstd-compressed body: %w", err)
	}
	defer func() {
		zr.Close()
	}()
	tr := tar.NewReader(zr)
	loggedChtimesError := false
	for {
		f, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar error: %w", err)
		}
		if !validRelPath(f.Name) {
			return fmt.Errorf("tar contained invalid name %q", f.Name)
		}
		rel := filepath.FromSlash(f.Name)
		abs := filepath.Join(dir, rel)

		fi := f.FileInfo()
		mode := fi.Mode()
		switch {
		case mode.IsRegular():
			parent := filepath.Dir(abs)
			if !madeDir[parent] {
				if err := os.MkdirAll(parent, 0755); err != nil { // #nosec G301 -- embed bundle dirs must be traversable for runtime
					return err
				}
				madeDir[parent] = true
			}
			wf, err := os.OpenFile(abs, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode.Perm()) // #nosec G304 -- abs under validated tar entry + fixed extract dir
			if err != nil {
				return err
			}
			n, err := io.Copy(wf, tr) // #nosec G110 -- trusted signed embed bundle; zstd decoder memory capped
			if closeErr := wf.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
			if err != nil {
				return fmt.Errorf("error writing to %s: %w", abs, err)
			}
			if n != f.Size {
				return fmt.Errorf("only wrote %d bytes to %s; expected %d", n, abs, f.Size)
			}
			modTime := f.ModTime
			if modTime.After(t0) {
				modTime = t0
			}
			if !modTime.IsZero() {
				if err := os.Chtimes(abs, modTime, modTime); err != nil && !loggedChtimesError {
					loggedChtimesError = true
				}
			}
			nFiles++
		case mode.IsDir():
			if err := os.MkdirAll(abs, 0755); err != nil { // #nosec G301 -- embed bundle dirs must be traversable for runtime
				return err
			}
			madeDir[abs] = true
		case mode&os.ModeSymlink != 0:
			if err := os.Symlink(f.Linkname, abs); err != nil {
				return err
			}
		default:
			return fmt.Errorf("tar file entry %s contained unsupported file type %v", f.Name, mode)
		}
	}
	return nil
}

func validRelPath(p string) bool {
	if p == "" || strings.Contains(p, `\`) || strings.HasPrefix(p, "/") || strings.Contains(p, "../") {
		return false
	}
	return true
}
