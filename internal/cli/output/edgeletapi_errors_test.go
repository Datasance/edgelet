package output

import "testing"

func TestFormatEdgeletAPIErrorMessageExecStartTimeout(t *testing.T) {
	got := FormatEdgeletAPIErrorMessage("EXEC_START_TIMEOUT", "exec start timeout after 15s")
	if got != execStartTimeoutMessage {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestFormatEdgeletAPIErrorMessagePassthrough(t *testing.T) {
	const server = "microservice not found"
	if got := FormatEdgeletAPIErrorMessage("NOT_FOUND", server); got != server {
		t.Fatalf("expected passthrough %q, got %q", server, got)
	}
}
