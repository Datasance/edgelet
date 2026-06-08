package run

import (
	"errors"
	"testing"
)

func TestExitCodeForCode(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{CodeInvalidArgument, ExitInvalidArgument},
		{CodeUnauthorized, ExitUnauthorized},
		{CodeForbidden, ExitUnauthorized},
		{CodeNotFound, ExitNotFound},
		{CodeConflict, ExitConflict},
		{CodeNotImplemented, ExitNotImplemented},
		{CodeDaemonUnavailable, ExitDaemonUnavailable},
		{"UNKNOWN", ExitInternal},
		{"", ExitInternal},
	}
	for _, tc := range tests {
		if got := ExitCodeForCode(tc.code); got != tc.want {
			t.Fatalf("ExitCodeForCode(%q) = %d, want %d", tc.code, got, tc.want)
		}
	}
}

func TestCLIErrorExitCode(t *testing.T) {
	err := NewCLIError(CodeDaemonUnavailable, "daemon not running", nil)
	if err.ExitCode() != ExitDaemonUnavailable {
		t.Fatalf("expected exit %d, got %d", ExitDaemonUnavailable, err.ExitCode())
	}
	if err.Error() != "Error[DAEMON_UNAVAILABLE]: daemon not running" {
		t.Fatalf("unexpected error string: %s", err.Error())
	}
}

func TestExitCodeForErrorUsesExitCoder(t *testing.T) {
	err := NewCLIError(CodeNotFound, "missing", nil)
	if got := ExitCodeForError(err); got != ExitNotFound {
		t.Fatalf("expected exit %d, got %d", ExitNotFound, got)
	}
}

func TestExitCodeForErrorFallbackInternal(t *testing.T) {
	if got := ExitCodeForError(errors.New("boom")); got != ExitInternal {
		t.Fatalf("expected exit %d, got %d", ExitInternal, got)
	}
}

func TestExitCodeForErrorNil(t *testing.T) {
	if got := ExitCodeForError(nil); got != ExitSuccess {
		t.Fatalf("expected exit 0, got %d", got)
	}
}
