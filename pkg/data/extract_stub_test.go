//go:build !linux || !full

package data

import (
	"bytes"
	"errors"
	"os"
)

func extractBinary(data []byte, destFile string) error {
	if existing, err := os.ReadFile(destFile); err == nil {
		if bytes.Equal(existing, data) {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmpFile := destFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0755); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, destFile); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}
	return nil
}
