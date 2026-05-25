//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package flock

// Acquire is a no-op on platforms without flock support.
func Acquire(path string) (int, error) {
	return -1, nil
}

// Release is a no-op on platforms without flock support.
func Release(lock int) error {
	return nil
}
