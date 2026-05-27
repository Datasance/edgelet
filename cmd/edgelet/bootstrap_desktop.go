//go:build !linux

package main

import (
	"fmt"

	edgeletcontainerdd "github.com/datasance/edgelet/pkg/containerd"
)

func startEmbeddedContainerdWithRetry() (*edgeletcontainerdd.Service, error) {
	return nil, fmt.Errorf("embedded containerd bootstrap is only available on linux")
}
