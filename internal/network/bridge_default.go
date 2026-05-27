//go:build !lite && !full

package network

func (m *Manager) getCNIBridgeInterfaceName() string {
	return ""
}
