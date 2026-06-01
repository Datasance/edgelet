package cgroups

import (
	"errors"
	"testing"
)

func TestSnapshotFromPolicyNil(t *testing.T) {
	snap := SnapshotFromPolicy(nil)
	if snap.Mode != string(ModeUnknown) {
		t.Fatalf("mode = %q", snap.Mode)
	}
	if snap.Driver != string(DriverCgroupfs) {
		t.Fatalf("driver = %q", snap.Driver)
	}
}

func TestSetGetGlobalPolicy(t *testing.T) {
	prev := GetGlobalPolicy()
	t.Cleanup(func() { SetGlobalPolicy(prev) })

	p := &CgroupPolicy{Mode: ModeV2, Driver: DriverSystemd}
	SetGlobalPolicy(p)
	got := GetGlobalPolicy()
	if got != p {
		t.Fatalf("GetGlobalPolicy returned unexpected pointer")
	}
}

func TestJoinControllersSorted(t *testing.T) {
	got := joinControllers([]string{"pids", "cpu", "memory"})
	want := "cpu,memory,pids"
	if got != want {
		t.Fatalf("joinControllers = %q want %q", got, want)
	}
}

func TestErrDelegationNestedMessage(t *testing.T) {
	err := &ErrDelegation{Controller: "cpu", Nested: true, Mode: ModeV2}
	if err.Error() == "" || !stringsContainsFold(err.Error(), "privileged") {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
}

func TestMapRuntimeErrorPassthrough(t *testing.T) {
	base := errors.New("unrelated failure")
	if got := MapRuntimeError(base, nil); got != base {
		t.Fatalf("expected passthrough")
	}
}
