package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeVersionResponse_FlatV38(t *testing.T) {
	raw := map[string]any{
		"versionCommand": "upgrade",
		"provisionKey":   "key-1",
		"expirationTime": float64(1718380800000),
		"semver":         "1.0.0-beta.3",
	}

	action, err := NormalizeVersionResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action["command"] != "UPGRADE" {
		t.Fatalf("command=%v", action["command"])
	}
	if action["provisionKey"] != "key-1" {
		t.Fatalf("provisionKey=%v", action["provisionKey"])
	}
	if action["semver"] != "1.0.0-beta.3" {
		t.Fatalf("semver=%v", action["semver"])
	}
}

func TestNormalizeVersionResponse_LegacyNested(t *testing.T) {
	raw := map[string]any{
		"versionCommand": map[string]any{
			"command":        "ROLLBACK",
			"version":        "v1.2.3",
			"provisionKey":   "key-2",
			"expirationTime": "1718380800000",
		},
	}

	action, err := NormalizeVersionResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action["command"] != "ROLLBACK" {
		t.Fatalf("command=%v", action["command"])
	}
	if action["version"] != "v1.2.3" {
		t.Fatalf("version=%v", action["version"])
	}
	if action["provisionKey"] != "key-2" {
		t.Fatalf("provisionKey=%v", action["provisionKey"])
	}
}

func TestNormalizeVersionResponse_AcceptsLowercaseCommand(t *testing.T) {
	action, err := NormalizeVersionResponse(map[string]any{"versionCommand": "rollback"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action["command"] != "ROLLBACK" {
		t.Fatalf("command=%v", action["command"])
	}
}

func TestTargetVersionFromAction_SemverPrecedence(t *testing.T) {
	action := map[string]any{
		"semver":  "2.0.0",
		"version": "1.0.0",
		"target":  "9.9.9",
	}
	if got := TargetVersionFromAction(action); got != "2.0.0" {
		t.Fatalf("expected semver precedence, got %q", got)
	}
}

func TestVersionForInstallScript_AddsVPrefix(t *testing.T) {
	if got := versionForInstallScript("1.0.0-beta.3"); got != "v1.0.0-beta.3" {
		t.Fatalf("got %q", got)
	}
	if got := versionForInstallScript("v2.0.0"); got != "v2.0.0" {
		t.Fatalf("got %q", got)
	}
}

func TestParseExpirationTimeMS(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want int64
	}{
		{"float64", float64(1718380800000), 1718380800000},
		{"int64", int64(1718380800000), 1718380800000},
		{"string", "1718380800000", 1718380800000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseExpirationTimeMS(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.UnixMilli() != tc.want {
				t.Fatalf("got %d want %d", got.UnixMilli(), tc.want)
			}
		})
	}
}

func TestOTAReprovisionPendingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ota-reprovision-pending")
	SetOTAReprovisionPendingPath(path)
	t.Cleanup(func() { SetOTAReprovisionPendingPath("") })

	expiry := time.UnixMilli(1718380800000)
	if err := WriteOTAReprovisionPending("key-3", "upgrade", "1.0.0", expiry); err != nil {
		t.Fatalf("write pending: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat pending: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}

	got, err := ReadOTAReprovisionPending()
	if err != nil {
		t.Fatalf("read pending: %v", err)
	}
	if got.ProvisionKey != "key-3" || got.Command != "upgrade" || got.TargetVersion != "1.0.0" {
		t.Fatalf("unexpected pending: %+v", got)
	}

	if err := DeleteOTAReprovisionPending(); err != nil {
		t.Fatalf("delete pending: %v", err)
	}
	if pending, err := ReadOTAReprovisionPending(); err != nil || pending != nil {
		t.Fatalf("expected nil pending after delete, got %+v err=%v", pending, err)
	}
}

func TestPendingOTAReprovision_ExpirySkew(t *testing.T) {
	expiry := time.UnixMilli(1718380800000)
	pending := &PendingOTAReprovision{ExpirationTime: expiry}

	if pending.IsExpired(expiry.Add(29 * time.Second)) {
		t.Fatal("expected valid within skew buffer")
	}
	if !pending.IsExpired(expiry.Add(31 * time.Second)) {
		t.Fatal("expected expired after skew buffer")
	}
}

func TestPendingFromAction(t *testing.T) {
	action := map[string]any{
		"command":        "UPGRADE",
		"provisionKey":   "abc",
		"expirationTime": json.Number("1718380800000"),
		"semver":         "1.0.0",
	}
	pending, err := PendingFromAction(action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending.TargetVersion != "1.0.0" {
		t.Fatalf("targetVersion=%q", pending.TargetVersion)
	}
}

func TestGetCandidateVersion_PrefersSemver(t *testing.T) {
	rm := NewReleaseManager(WithRunningVersion(func() string { return "1.0.0" }))
	got, err := rm.GetCandidateVersion(map[string]any{"semver": "v2.0.0", "version": "9.9.9"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2.0.0" {
		t.Fatalf("got %q", got)
	}
}
