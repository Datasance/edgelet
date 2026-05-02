//go:build linux

package cri

import (
	"testing"

	"github.com/eclipse-iofog/agent/internal/models"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func strPtr(s string) *string { return &s }

func TestLinuxNamespaceOptionsFromMicroservice(t *testing.T) {
	ms := models.NewMicroservice("u1", "img")
	opts := linuxNamespaceOptionsFromMicroservice(ms)
	if opts.Network != runtimeapi.NamespaceMode_POD || opts.Pid != runtimeapi.NamespaceMode_CONTAINER || opts.Ipc != runtimeapi.NamespaceMode_CONTAINER {
		t.Fatalf("default: got %+v", opts)
	}

	ms.HostNetworkMode = true
	opts = linuxNamespaceOptionsFromMicroservice(ms)
	if opts.Network != runtimeapi.NamespaceMode_NODE {
		t.Fatalf("host network: Network=%v", opts.Network)
	}

	ms.HostNetworkMode = false
	ms.PidMode = strPtr("host")
	ms.IpcMode = strPtr("host")
	opts = linuxNamespaceOptionsFromMicroservice(ms)
	if opts.Pid != runtimeapi.NamespaceMode_NODE || opts.Ipc != runtimeapi.NamespaceMode_NODE {
		t.Fatalf("host pid/ipc: got %+v", opts)
	}
}

func TestPodSandboxNeedsLinuxBlock(t *testing.T) {
	if podSandboxNeedsLinuxBlock(nil) {
		t.Fatal("nil should be false")
	}
	ms := models.NewMicroservice("u1", "img")
	if podSandboxNeedsLinuxBlock(ms) {
		t.Fatal("plain ms should be false")
	}
	ms.IsPrivileged = true
	if !podSandboxNeedsLinuxBlock(ms) {
		t.Fatal("privileged should need linux block")
	}
	ms.IsPrivileged = false
	ms.HostNetworkMode = true
	if !podSandboxNeedsLinuxBlock(ms) {
		t.Fatal("host network should need linux block")
	}
	ms.HostNetworkMode = false
	ms.PidMode = strPtr("host")
	if !podSandboxNeedsLinuxBlock(ms) {
		t.Fatal("host pid should need linux block")
	}
}

func TestGetRuntimeHandler(t *testing.T) {
	if GetRuntimeHandler(nil) != RuntimeHandlerRunc {
		t.Fatal("nil -> runc")
	}
	ms := models.NewMicroservice("u1", "img")
	ms.Runtime = strPtr("spin")
	if GetRuntimeHandler(ms) != RuntimeHandlerSpin {
		t.Fatal("spin only when safe")
	}
	ms.IsPrivileged = true
	if GetRuntimeHandler(ms) != RuntimeHandlerRunc {
		t.Fatal("privileged forces runc")
	}
	ms.IsPrivileged = false
	ms.Runtime = strPtr("spin")
	ms.HostNetworkMode = true
	if GetRuntimeHandler(ms) != RuntimeHandlerRunc {
		t.Fatal("host network forces runc")
	}
}
