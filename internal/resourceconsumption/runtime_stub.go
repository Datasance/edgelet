//go:build !linux

package resourceconsumption

func (rcm *Manager) embeddedRuntimePIDs() []int {
	if rcm.runtimePIDReader != nil {
		return rcm.runtimePIDReader()
	}
	return nil
}
