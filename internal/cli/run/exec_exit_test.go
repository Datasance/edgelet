package run

import "testing"

func TestExecExitErrorExitCode(t *testing.T) {
	err := NewExecExitError(42)
	if got := ExitCodeForError(err); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}
