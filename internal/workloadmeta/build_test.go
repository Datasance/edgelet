package workloadmeta

import "testing"

func TestBuildLabelsCanonicalAndProtected(t *testing.T) {
	in := BuildInput{
		MicroserviceUUID: "ms-1",
		MicroserviceName: "video-analyzer",
		ApplicationName:  LocalDeployApplicationName,
		NodeUUID:         "node-1",
		RuntimeEngine:    "DOCKER",
		IsRouter:         true,
		IsNats:           true, // router precedence should win
		HostNetwork:      false,
		IsSystem:         true,
		SandboxID:        "sandbox-1",
		UserLabels: map[string]string{
			"custom.label":                       "ok",
			LabelMicroserviceUID:                 "override-disallowed",
			"APP.KUBERNETES.IO/NAME":             "override-disallowed",
			"  edgelet.iofog.org/runtime-engine": "override-disallowed",
		},
	}

	got := BuildLabels(in)

	if got[LabelMicroserviceUID] != "ms-1" {
		t.Fatalf("expected canonical microservice uid, got %q", got[LabelMicroserviceUID])
	}
	if got[LabelRole] != RoleRouter {
		t.Fatalf("expected router role, got %q", got[LabelRole])
	}
	if got[LabelScope] != ScopeLocal {
		t.Fatalf("expected local scope, got %q", got[LabelScope])
	}
	if got[LabelRuntimeEngine] != RuntimeEngineDocker {
		t.Fatalf("expected normalized runtime engine %q, got %q", RuntimeEngineDocker, got[LabelRuntimeEngine])
	}
	if got[LabelSystem] != "true" {
		t.Fatalf("expected system label true, got %q", got[LabelSystem])
	}
	if got[LabelHostNetwork] != "false" {
		t.Fatalf("expected host-network false, got %q", got[LabelHostNetwork])
	}
	if got[LabelSandboxID] != "sandbox-1" {
		t.Fatalf("expected sandbox label, got %q", got[LabelSandboxID])
	}
	if got["custom.label"] != "ok" {
		t.Fatalf("expected custom user label to survive, got %q", got["custom.label"])
	}
}

func TestBuildEnvReservedAndTZPolicy(t *testing.T) {
	in := BuildInput{
		MicroserviceUUID: "ms-2",
		MicroserviceName: "svc",
		ApplicationName:  "app",
		NodeUUID:         "node-2",
		RuntimeEngine:    "podman",
		IsRouter:         false,
		IsNats:           true,
		HostNetwork:      false,
		TimeZone:         "Europe/Istanbul",
		UserEnv: map[string]string{
			EnvMicroserviceUID: "override-disallowed",
			"USER_A":           "a",
			EnvTimeZone:        "Asia/Tokyo",
		},
	}

	got := BuildEnv(in)

	wantPrefix := []string{
		EnvMicroserviceUID + "=ms-2",
		EnvMicroserviceName + "=svc",
		EnvApplicationName + "=app",
		EnvNodeUID + "=node-2",
		EnvScope + "=" + ScopeManaged,
		EnvRuntimeEngine + "=" + RuntimeEnginePodman,
		EnvRole + "=" + RoleNats,
	}
	if len(got) < len(wantPrefix) {
		t.Fatalf("expected at least %d env vars, got %d", len(wantPrefix), len(got))
	}
	for i, expected := range wantPrefix {
		if got[i] != expected {
			t.Fatalf("env[%d] expected %q, got %q", i, expected, got[i])
		}
	}

	// Reserved key should not be user-overridden.
	for _, entry := range got {
		if entry == EnvMicroserviceUID+"=override-disallowed" {
			t.Fatal("reserved env key was overridden")
		}
	}

	// User TZ should be preserved and no injected TZ should be added.
	tzCount := 0
	for _, entry := range got {
		if entry == EnvTimeZone+"=Asia/Tokyo" {
			tzCount++
		}
		if entry == EnvTimeZone+"=Europe/Istanbul" {
			t.Fatal("injected timezone should not be present when user TZ exists")
		}
	}
	if tzCount != 1 {
		t.Fatalf("expected one user TZ entry, got %d", tzCount)
	}
}

func TestBuildEnvInjectTZWhenMissing(t *testing.T) {
	in := BuildInput{
		MicroserviceUUID: "ms-3",
		MicroserviceName: "svc",
		ApplicationName:  "app",
		NodeUUID:         "node-3",
		RuntimeEngine:    "edgelet",
		TimeZone:         "",
		UserEnv: map[string]string{
			"USER_B": "b",
		},
	}

	got := BuildEnv(in)
	found := false
	for _, entry := range got {
		if entry == EnvTimeZone+"=UTC" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected default UTC timezone injection")
	}
}

func TestMergeUserLabelsCanonicalWins(t *testing.T) {
	user := map[string]string{
		"custom.one":             "1",
		LabelScope:               "override",
		"APP.KUBERNETES.IO/NAME": "override",
	}
	canonical := map[string]string{
		LabelScope: ScopeManaged,
	}

	got := MergeUserLabels(user, canonical)
	if got["custom.one"] != "1" {
		t.Fatalf("expected custom label preserved, got %q", got["custom.one"])
	}
	if got[LabelScope] != ScopeManaged {
		t.Fatalf("expected canonical scope to win, got %q", got[LabelScope])
	}

	// Ensure protected user key didn't leak.
	if _, exists := got["app.kubernetes.io/name"]; exists {
		t.Fatal("protected app label from user input should not be present without canonical value")
	}
}
