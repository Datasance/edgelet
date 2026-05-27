//go:build lite || full

package network

import "github.com/datasance/edgelet/internal/constants"

// getCNIBridgeInterfaceName returns the managed CNI bridge interface name.
// Full and lite use the same edgelet bridge naming (edgelet0).
func (m *Manager) getCNIBridgeInterfaceName() string {
	return constants.EdgeletBridgeName
}
