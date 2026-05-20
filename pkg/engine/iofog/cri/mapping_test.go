//go:build linux

package cri

import (
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/workloadmeta"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func strPtr(s string) *string { return &s }

func assertNoRemovedLegacyLabels(t *testing.T, labels map[string]string) {
	t.Helper()
	for _, k := range workloadmeta.RemovedLegacyLabelKeys {
		if v, ok := labels[k]; ok && strings.TrimSpace(v) != "" {
			t.Fatalf("legacy label key %q must not be set, got %q", k, v)
		}
	}
}

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
	if GetRuntimeHandler(nil) != RuntimeHandlerCrun {
		t.Fatal("nil -> crun")
	}
	ms := models.NewMicroservice("u1", "img")
	ms.IsPrivileged = true
	if GetRuntimeHandler(ms) != RuntimeHandlerCrun {
		t.Fatal("privileged forces crun")
	}
	ms.IsPrivileged = false
	ms.HostNetworkMode = true
	if GetRuntimeHandler(ms) != RuntimeHandlerCrun {
		t.Fatal("host network forces crun")
	}
	ms.HostNetworkMode = false
	ms.ApplicationName = "local"
	ms.Runtime = nil
	if GetRuntimeHandler(ms) != RuntimeHandlerCrunLocal {
		t.Fatal("local non-host workload should use crun-local")
	}
}

func TestPodSandboxConfigFromMicroserviceSetsNetworkAnnotation(t *testing.T) {
	ms := models.NewMicroservice("u1", "img")
	ms.ApplicationName = "local"
	cfg := PodSandboxConfigFromMicroservice(ms, "127.0.0.1", "/tmp/logs", "node-1")
	if cfg.Annotations["iofog.network"] != "iofog-local" {
		t.Fatalf("expected local annotation iofog-local, got %q", cfg.Annotations["iofog.network"])
	}
	ms.ApplicationName = "managed"
	cfg = PodSandboxConfigFromMicroservice(ms, "127.0.0.1", "/tmp/logs", "node-1")
	if cfg.Annotations["iofog.network"] != "iofog" {
		t.Fatalf("expected managed annotation iofog, got %q", cfg.Annotations["iofog.network"])
	}
}

func TestPodSandboxConfigFromMicroserviceUsesCanonicalLabels(t *testing.T) {
	ms := models.NewMicroservice("u1", "img")
	ms.MicroserviceName = "svc"
	ms.ApplicationName = "app"
	ms.IsRouter = true
	ms.IsNats = true // router precedence

	cfg := PodSandboxConfigFromMicroservice(ms, "127.0.0.1", "/tmp/logs", "node-1")

	if cfg.Labels[workloadmeta.LabelNodeUID] != "node-1" {
		t.Fatalf("expected canonical node uid label, got %q", cfg.Labels[workloadmeta.LabelNodeUID])
	}
	if cfg.Labels[workloadmeta.LabelMicroserviceUID] != "u1" {
		t.Fatalf("expected canonical microservice uid label, got %q", cfg.Labels[workloadmeta.LabelMicroserviceUID])
	}
	if cfg.Labels[workloadmeta.LabelRuntimeEngine] != workloadmeta.RuntimeEngineIofog {
		t.Fatalf("expected runtime-engine iofog, got %q", cfg.Labels[workloadmeta.LabelRuntimeEngine])
	}
	if cfg.Labels[workloadmeta.LabelRole] != workloadmeta.RoleRouter {
		t.Fatalf("expected router role precedence, got %q", cfg.Labels[workloadmeta.LabelRole])
	}
	assertNoRemovedLegacyLabels(t, cfg.Labels)
}

func TestContainerConfigFromMicroserviceUsesCanonicalLabels(t *testing.T) {
	ms := models.NewMicroservice("u2", "img")
	ms.MicroserviceName = "svc2"
	ms.ApplicationName = "local"
	ms.HostNetworkMode = false

	cfg, err := ContainerConfigFromMicroservice(ms, "127.0.0.1", []string{}, "0.log", "", "", "sandbox-123", "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Labels[workloadmeta.LabelMicroserviceUID] != "u2" {
		t.Fatalf("expected canonical microservice uid label, got %q", cfg.Labels[workloadmeta.LabelMicroserviceUID])
	}
	if cfg.Labels[workloadmeta.LabelScope] != workloadmeta.ScopeLocal {
		t.Fatalf("expected local scope, got %q", cfg.Labels[workloadmeta.LabelScope])
	}
	if cfg.Labels[workloadmeta.LabelSandboxID] != "sandbox-123" {
		t.Fatalf("expected canonical sandbox id label, got %q", cfg.Labels[workloadmeta.LabelSandboxID])
	}
	if cfg.Labels[workloadmeta.LabelNodeUID] != "node-1" {
		t.Fatalf("expected canonical node uid label, got %q", cfg.Labels[workloadmeta.LabelNodeUID])
	}
	assertNoRemovedLegacyLabels(t, cfg.Labels)
}
