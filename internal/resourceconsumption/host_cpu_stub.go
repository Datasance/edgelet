//go:build !linux

package resourceconsumption

func (rcm *Manager) getTotalCPULinux() float64 {
	return 0
}
