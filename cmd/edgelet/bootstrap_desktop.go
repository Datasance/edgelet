//go:build !linux

package main

import (
	"errors"

	"github.com/eclipse-iofog/edgelet/pkg/containerd"
)

func startEmbeddedContainerdWithRetry() (*containerd.Service, error) {
	return nil, errors.New("embedded containerd bootstrap is only available on linux")
}
