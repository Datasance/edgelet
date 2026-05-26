//go:build linux

package cri

import (
	"errors"
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/workloadmeta"
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
	handler, err := ResolveRuntimeHandler(nil, nil)
	if err != nil {
		t.Fatalf("nil runtime handler resolve error: %v", err)
	}
	if handler != RuntimeHandlerCrun {
		t.Fatal("nil -> crun")
	}
	ms := models.NewMicroservice("u1", "img")
	ms.IsPrivileged = true
	handler, err = ResolveRuntimeHandler(ms, nil)
	if err != nil {
		t.Fatalf("privileged runtime handler resolve error: %v", err)
	}
	if handler != RuntimeHandlerCrun {
		t.Fatal("privileged forces crun")
	}
	ms.IsPrivileged = false
	ms.HostNetworkMode = true
	handler, err = ResolveRuntimeHandler(ms, nil)
	if err != nil {
		t.Fatalf("host net runtime handler resolve error: %v", err)
	}
	if handler != RuntimeHandlerCrun {
		t.Fatal("host network forces crun")
	}
	ms.HostNetworkMode = false
	ms.ApplicationName = "edgelet"
	ms.Runtime = nil
	handler, err = ResolveRuntimeHandler(ms, nil)
	if err != nil {
		t.Fatalf("local runtime handler resolve error: %v", err)
	}
	if handler != RuntimeHandlerCrun {
		t.Fatal("local non-host workload should use canonical crun")
	}
}

func TestResolveRuntimeHandler_UsesCanonicalRuntimeNames(t *testing.T) {
	runtimeClasses := []*models.LocalRuntimeClass{
		{
			Name:        "edgelet",
			Handler:     "edgelet",
			RuntimeName: "edgelet",
		},
	}

	managed := models.NewMicroservice("u1", "img")
	runtime := "edgelet"
	managed.Runtime = &runtime
	handler, err := ResolveRuntimeHandler(managed, runtimeClasses)
	if err != nil {
		t.Fatalf("managed runtime resolve error: %v", err)
	}
	if handler != "edgelet" {
		t.Fatalf("expected edgelet runtime handler, got %q", handler)
	}

	local := models.NewMicroservice("u2", "img")
	local.ApplicationName = "edgelet"
	local.Runtime = &runtime
	handler, err = ResolveRuntimeHandler(local, runtimeClasses)
	if err != nil {
		t.Fatalf("local runtime resolve error: %v", err)
	}
	if handler != "edgelet" {
		t.Fatalf("expected edgelet runtime handler for local workload, got %q", handler)
	}
}

func TestResolveRuntimeHandler_RejectsUnknownRuntimeClass(t *testing.T) {
	ms := models.NewMicroservice("u1", "img")
	runtime := "unknown-runtime"
	ms.Runtime = &runtime
	_, err := ResolveRuntimeHandler(ms, nil)
	if err == nil {
		t.Fatal("expected unknown runtime class error")
	}
	if !errors.Is(err, ErrUnknownRuntimeClass) {
		t.Fatalf("expected ErrUnknownRuntimeClass, got %v", err)
	}
}

func TestPodSandboxConfigFromMicroserviceSetsNetworkAnnotation(t *testing.T) {
	ms := models.NewMicroservice("u1", "img")
	ms.ApplicationName = "edgelet"
	cfg := PodSandboxConfigFromMicroservice(ms, "127.0.0.1", "/tmp/logs", "node-1")
	if cfg.Annotations[AnnotationIofogNetwork] != "edgelet" {
		t.Fatalf("expected local annotation iofog in single-bridge mode, got %q", cfg.Annotations[AnnotationIofogNetwork])
	}
	ms.HostNetworkMode = true
	cfg = PodSandboxConfigFromMicroservice(ms, "127.0.0.1", "/tmp/logs", "node-1")
	if cfg.Annotations[AnnotationIofogNetwork] != "edgelet" {
		t.Fatalf("expected host-network local workload to bypass local annotation, got %q", cfg.Annotations[AnnotationIofogNetwork])
	}
	ms.HostNetworkMode = false
	ms.ApplicationName = "managed"
	cfg = PodSandboxConfigFromMicroservice(ms, "127.0.0.1", "/tmp/logs", "node-1")
	if cfg.Annotations[AnnotationIofogNetwork] != "edgelet" {
		t.Fatalf("expected managed annotation iofog, got %q", cfg.Annotations[AnnotationIofogNetwork])
	}
}

func TestSelectCNINetworkForMicroservice_RuntimeIndependent(t *testing.T) {
	ms := models.NewMicroservice("u3", "img")
	ms.ApplicationName = "edgelet"
	requestedRuntime := "edgelet"
	ms.Runtime = &requestedRuntime

	selection := SelectCNINetworkForMicroservice(ms)
	if selection.Scope != workloadmeta.ScopeLocal {
		t.Fatalf("expected local scope, got %q", selection.Scope)
	}
	if selection.NetworkName != "edgelet" {
		t.Fatalf("expected canonical iofog network in single-bridge mode, got %q", selection.NetworkName)
	}

	ms.HostNetworkMode = true
	selection = SelectCNINetworkForMicroservice(ms)
	if selection.Scope != workloadmeta.ScopeManaged {
		t.Fatalf("expected managed scope on host network, got %q", selection.Scope)
	}
	if selection.NetworkName != "edgelet" {
		t.Fatalf("expected managed network on host network bypass, got %q", selection.NetworkName)
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
	if cfg.Labels[workloadmeta.LabelRuntimeEngine] != workloadmeta.RuntimeEngineEdgelet {
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
	ms.ApplicationName = "edgelet"
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
