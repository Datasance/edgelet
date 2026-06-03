//go:build linux

package cri

import (
	"errors"
	"testing"
)

func TestReservedPodSandboxID(t *testing.T) {
	err := errors.New(`RunPodSandbox: rpc error: code = FailedPrecondition desc = failed to reserve sandbox name "edgelet_abc_0": name "edgelet_abc_0" is reserved for "sandbox-123"`)
	if !IsPodSandboxNameReserved(err) {
		t.Fatal("expected reservation error")
	}
	if got := ReservedPodSandboxID(err); got != "sandbox-123" {
		t.Fatalf("expected sandbox-123, got %q", got)
	}
}

func TestReservedPodSandboxID_NoMatch(t *testing.T) {
	err := errors.New("some other error")
	if IsPodSandboxNameReserved(err) {
		t.Fatal("expected false")
	}
	if got := ReservedPodSandboxID(err); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
