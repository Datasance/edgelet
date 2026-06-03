//go:build linux && cgo

package cgroups

import "testing"

func TestPublishHostPolicy_setsGlobalPolicy(t *testing.T) {
	prev := GetGlobalPolicy()
	t.Cleanup(func() { SetGlobalPolicy(prev) })
	SetGlobalPolicy(nil)

	policy, err := PublishHostPolicy()
	if err != nil {
		t.Fatalf("PublishHostPolicy failed: %v", err)
	}
	if policy == nil {
		t.Fatal("expected non-nil policy")
	}
	if policy.Mode == ModeUnknown {
		t.Fatalf("expected detectable mode, got unknown")
	}
	got := GetGlobalPolicy()
	if got != policy {
		t.Fatal("GetGlobalPolicy should return published policy")
	}
	snap := GetSnapshot()
	if snap.Mode != string(policy.Mode) {
		t.Fatalf("snapshot mode = %q want %q", snap.Mode, policy.Mode)
	}
	if snap.Driver != string(policy.Driver) {
		t.Fatalf("snapshot driver = %q want %q", snap.Driver, policy.Driver)
	}
	if snap.Nested != policy.Nested {
		t.Fatalf("snapshot nested = %t want %t", snap.Nested, policy.Nested)
	}
}
