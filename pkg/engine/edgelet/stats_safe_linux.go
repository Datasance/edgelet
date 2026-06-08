//go:build linux

package edgelet

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

func clampUint64ToInt64(v uint64) int64 {
	if v > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(v)
}

func pathUnderBase(base, name string) (string, error) {
	full := filepath.Clean(filepath.Join(base, name))
	cleanBase := filepath.Clean(base)
	if full != cleanBase && !strings.HasPrefix(full, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes base %q", name, base)
	}
	return full, nil
}
