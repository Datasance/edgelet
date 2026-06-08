package engine

import (
	"errors"
	"testing"
)

func TestIsNonRestartableContainerError(t *testing.T) {
	inner := &NonRestartableContainerError{
		ContainerID: "cid-1",
		Reason:      CRIReasonContainerExited,
		ExitCode:    255,
		Message:     "container is in CONTAINER_EXITED state",
	}
	wrapped := errors.New("wrapper: " + inner.Error())
	if _, ok := IsNonRestartableContainerError(wrapped); ok {
		t.Fatal("expected false for non-wrapped error")
	}

	err := errors.Join(errors.New("wrapper"), inner)
	got, ok := IsNonRestartableContainerError(err)
	if !ok {
		t.Fatal("expected typed non-restartable error")
	}
	if got.Reason != CRIReasonContainerExited {
		t.Fatalf("expected reason %q, got %q", CRIReasonContainerExited, got.Reason)
	}
}

func TestIsNonRestartableCRIReason(t *testing.T) {
	if !IsNonRestartableCRIReason("CONTAINER_EXITED") {
		t.Fatal("expected CONTAINER_EXITED to be non-restartable")
	}
	if !IsNonRestartableCRIReason("container_exited") {
		t.Fatal("expected case-insensitive match")
	}
	if IsNonRestartableCRIReason("OOMKILLED") {
		t.Fatal("did not expect OOMKILLED to be non-restartable")
	}
}
