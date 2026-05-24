//go:build (linux && amd64) || (linux && arm64) || (linux && arm) || (linux && riscv64)

package embedded

import (
	"github.com/datasance/edgelet/internal/constants"
)

// generateCNIConfig builds a CNI conflist for one bridge network.
func generateCNIConfig(networkName, bridgeName, bridgeCIDR string) map[string]any {
	return map[string]any{
		"cniVersion": "1.0.0",
		"name":       networkName,
		"plugins": []map[string]any{
			{
				"type":        "bridge",
				"bridge":      bridgeName,
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
							{"subnet": bridgeCIDR},
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

func generateManagedCNIConfig() map[string]any {
	return generateCNIConfig(constants.EdgeletNetworkName, constants.EdgeletBridgeName, constants.EdgeletBridgeCIDR)
}
