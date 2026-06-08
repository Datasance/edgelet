//go:build linux && cgo

package cgroups

import (
	"fmt"

	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

// PublishHostPolicy runs Detect() and stores the result for status reporting
// without mutating the cgroup hierarchy. Used by the control plane when
// EDGELET_RUNTIME_SPLIT=1; bootstrap remains owned by runtime-bootstrap.
func PublishHostPolicy() (*CgroupPolicy, error) {
	policy, err := Detect()
	if err != nil {
		return nil, err
	}
	SetGlobalPolicy(policy)
	logging.LogInfo("Cgroups", fmt.Sprintf(
		"control plane recorded host policy mode=%s driver=%s nested=%t",
		policy.Mode, policy.Driver, policy.Nested,
	))
	return policy, nil
}
