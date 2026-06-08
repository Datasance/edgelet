//go:build !linux

package main

import (
	"errors"

	edgeletcontainerdd "github.com/datasance/edgelet/pkg/containerd"
)

func startEmbeddedContainerdWithRetry() (*edgeletcontainerdd.Service, error) {
	return nil, errors.New("embedded containerd bootstrap is only available on linux")
}
