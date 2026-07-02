//go:build linux

package containerd

// FindEmbeddedContainerdChildPIDs returns PIDs whose cmdline includes --edgelet-containerd-child.
func FindEmbeddedContainerdChildPIDs() ([]int, error) {
	return findContainerdChildPIDs()
}
