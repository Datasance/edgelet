//go:build (linux && amd64) || (linux && arm64) || (linux && arm) || (linux && riscv64)

package embedded

import (
	"github.com/eclipse-iofog/agent/internal/constants"
)

// generateCNIConfig returns the CNI conflist for the "iofog" bridge network.
// The bridge is named iofog0 to avoid collision with Docker's docker0 bridge.
// CIDR 172.18.0.0/16 is chosen to avoid Docker's default 172.17.0.0/16.
func generateCNIConfig() map[string]any {
	return map[string]any{
		"cniVersion": "1.0.0",
		"name":       constants.IofogNetworkName,
		"plugins": []map[string]any{
			{
				"type":        "bridge",
				"bridge":      constants.IofogBridgeName,
				"isGateway":   true,
				"ipMasq":      true,
				"hairpinMode": true,
				"capabilities": map[string]any{
					"portMappings": true,
					"ips":          true,
				},
				"ipam": map[string]any{
					"type": "host-local",
					"ranges": [][]map[string]any{
						{
							{"subnet": constants.IofogBridgeCIDR},
						},
					},
					"routes": []map[string]any{
						{"dst": "0.0.0.0/0"},
					},
				},
			},
			{
				"type": "portmap",
				"capabilities": map[string]any{
					"portMappings": true,
				},
			},
			{
				"type": "loopback",
			},
		},
	}
}
