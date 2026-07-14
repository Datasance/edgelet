package runtimecmd

import (
	"fmt"

	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/pkg/containerd"
)

// ReapOrphansResult carries the outcome of a local data-plane orphan reap.
type ReapOrphansResult struct {
	Human string
	Data  map[string]any
}

// ReapOrphans stops edgelet-scoped containerd shims and orphaned embedded containerd children.
func ReapOrphans() (*ReapOrphansResult, error) {
	if err := containerd.ReapManagedShimsUntilClear(constants.EdgeletContainerdSocket, containerd.DefaultShimReapBudgetCap); err != nil {
		return nil, err
	}
	data := map[string]any{
		"socket":  constants.EdgeletContainerdSocket,
		"status":  "complete",
		"message": "edgelet data-plane orphan reap complete",
	}
	return &ReapOrphansResult{
		Human: fmt.Sprintf("Data-plane orphan reap complete (%s).", constants.EdgeletContainerdSocket),
		Data:  data,
	}, nil
}
