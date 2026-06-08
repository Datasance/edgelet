//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package flock

import "golang.org/x/sys/unix"

// Acquire creates an exclusive lock on path for the process lifetime.
func Acquire(path string) (int, error) {
	lock, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC, 0600)
	if err != nil {
		return -1, err
	}
	return lock, unix.Flock(lock, unix.LOCK_EX)
}

// Release removes an existing lock held by this process.
func Release(lock int) error {
	if lock < 0 {
		return nil
	}
	return unix.Flock(lock, unix.LOCK_UN)
}
