//go:build linux

package resourceconsumption

import (
	"github.com/eclipse-iofog/edgelet/pkg/containerd"
)

func (rcm *Manager) embeddedRuntimePIDs() []int {
	if rcm.runtimePIDReader != nil {
		return rcm.runtimePIDReader()
	}
	pids, err := containerd.FindEmbeddedContainerdChildPIDs()
	if err != nil || len(pids) == 0 {
		return nil
	}
	return pids
}
