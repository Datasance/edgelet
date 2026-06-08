//go:build linux

package edgelet

import (
	"testing"

	"github.com/containerd/containerd/v2/core/content"
)

func TestComputePullProgressMonotonicAndCapped(t *testing.T) {
	statuses := []content.Status{
		{Ref: "a", Offset: 50, Total: 100},
		{Ref: "b", Offset: 20, Total: 100},
	}
	if got := computePullProgress(statuses, 0); got <= 0 {
		t.Fatalf("expected positive progress, got %f", got)
	}
	if got := computePullProgress(statuses, 80); got != 80 {
		t.Fatalf("expected monotonic progress to stay at prev when lower, got %f", got)
	}

	nearDone := []content.Status{{Ref: "x", Offset: 100, Total: 100}}
	if got := computePullProgress(nearDone, 0); got > 99 {
		t.Fatalf("expected progress cap at 99 before final callback, got %f", got)
	}
}
