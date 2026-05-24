package models

import (
	"path/filepath"
	"strings"

	"github.com/datasance/edgelet/internal/constants"
)

// LocalRuntimeClass is the persistent RuntimeClass row stored in SQLite.
type LocalRuntimeClass struct {
	Name        string `json:"name"`
	Handler     string `json:"handler"`
	RuntimeName string `json:"runtimeName,omitempty"`
	CreatedAt   int64  `json:"createdAt,omitempty"`
	UpdatedAt   int64  `json:"updatedAt,omitempty"`
}

func (r *LocalRuntimeClass) Normalize() {
	if r == nil {
		return
	}
	r.Name = strings.TrimSpace(strings.ToLower(r.Name))
	r.Handler = strings.TrimSpace(strings.ToLower(r.Handler))
	r.RuntimeName = strings.TrimSpace(strings.ToLower(r.RuntimeName))
	if r.RuntimeName == "" {
		r.RuntimeName = r.Name
	}
}

func (r *LocalRuntimeClass) RuntimeBinaryPath() string {
	if r == nil {
		return ""
	}
	return RuntimeClassBinaryPathForHandler(r.Handler)
}

func RuntimeClassBinaryPathForHandler(handler string) string {
	normalized := strings.TrimSpace(strings.ToLower(handler))
	if normalized == "" {
		return ""
	}
	binaryName := normalized
	if !strings.HasPrefix(binaryName, "containerd-shim-") {
		binaryName = "containerd-shim-" + normalized
	}
	return filepath.Join(constants.EdgeletContainerdBinDir, binaryName)
}
