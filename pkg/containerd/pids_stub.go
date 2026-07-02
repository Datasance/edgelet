//go:build !linux

package containerd

// FindEmbeddedContainerdChildPIDs is unsupported off Linux.
func FindEmbeddedContainerdChildPIDs() ([]int, error) {
	return nil, nil
}
