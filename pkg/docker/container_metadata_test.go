
package docker

import (
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/workloadmeta"
)

func TestBuildCanonicalContainerMetadataUsesCanonicalKeys(t *testing.T) {
	cfg := &config.Config{
		IOFogUUID: "node-123",
		TimeZone:  "Europe/Istanbul",
	}

	ms := models.NewMicroservice("ms-123", "repo/app:1")
	ms.MicroserviceName = "video-analyzer"
	ms.ApplicationName = "smart-city"
	ms.IsRouter = true
	ms.IsNats = true // router precedence must win
	ms.HostNetworkMode = false
	ms.EnvVars = []*models.EnvVar{
		{Key: "USER_ONE", Value: "value1"},
		{Key: workloadmeta.EnvRole, Value: "should-be-ignored"},
	}
	ms.Labels = map[string]string{
		"custom.owner":                  "edge-team",
		workloadmeta.LabelRuntimeEngine: "tampered",
		workloadmeta.LabelAppManagedBy:  "other",
	}

	labels, envs := buildCanonicalContainerMetadata(ms, cfg)

	if labels[workloadmeta.LabelMicroserviceUID] != "ms-123" {
		t.Fatalf("expected canonical microservice uid, got %q", labels[workloadmeta.LabelMicroserviceUID])
	}
	if labels[workloadmeta.LabelNodeUID] != "node-123" {
		t.Fatalf("expected canonical node uid, got %q", labels[workloadmeta.LabelNodeUID])
	}
	if labels[workloadmeta.LabelRuntimeEngine] != workloadmeta.RuntimeEngineDocker {
		t.Fatalf("expected runtime engine docker, got %q", labels[workloadmeta.LabelRuntimeEngine])
	}
	if labels[workloadmeta.LabelRole] != workloadmeta.RoleRouter {
		t.Fatalf("expected router role precedence, got %q", labels[workloadmeta.LabelRole])
	}
	if labels["custom.owner"] != "edge-team" {
		t.Fatalf("expected custom label to be preserved, got %q", labels["custom.owner"])
	}
	if labels[workloadmeta.LabelRuntimeEngine] != workloadmeta.RuntimeEngineDocker {
		t.Fatalf("expected canonical runtime label to win conflicts, got %q", labels[workloadmeta.LabelRuntimeEngine])
	}
	if labels[workloadmeta.LabelAppManagedBy] != workloadmeta.ManagedByValue {
		t.Fatalf("expected canonical managed-by label to win conflicts, got %q", labels[workloadmeta.LabelAppManagedBy])
	}
	for _, lk := range workloadmeta.RemovedLegacyLabelKeys {
		if v := strings.TrimSpace(labels[lk]); v != "" {
			t.Fatalf("removed legacy label %q must not be emitted, got %q", lk, v)
		}
	}

	assertEnvPresent(t, envs, workloadmeta.EnvMicroserviceUID, "ms-123")
	assertEnvPresent(t, envs, workloadmeta.EnvMicroserviceName, "video-analyzer")
	assertEnvPresent(t, envs, workloadmeta.EnvApplicationName, "smart-city")
	assertEnvPresent(t, envs, workloadmeta.EnvNodeUID, "node-123")
	assertEnvPresent(t, envs, workloadmeta.EnvRuntimeEngine, workloadmeta.RuntimeEngineDocker)
	assertEnvPresent(t, envs, workloadmeta.EnvRole, workloadmeta.RoleRouter)
	assertEnvPresent(t, envs, "USER_ONE", "value1")
	assertEnvPresent(t, envs, workloadmeta.EnvTimeZone, "Europe/Istanbul")

	removedEnv := map[string]struct{}{}
	for _, k := range workloadmeta.RemovedLegacyEnvVars {
		removedEnv[strings.ToUpper(strings.TrimSpace(k))] = struct{}{}
	}
	for _, env := range envs {
		idx := strings.Index(env, "=")
		if idx <= 0 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(env[:idx]))
		if _, bad := removedEnv[key]; bad {
			t.Fatalf("removed legacy env key must not be emitted: %s", env)
		}
	}
}

func assertEnvPresent(t *testing.T, envs []string, key, wantValue string) {
	t.Helper()
	want := key + "=" + wantValue
	for _, env := range envs {
		if env == want {
			return
		}
	}
	t.Fatalf("expected env %q in %v", want, envs)
}
