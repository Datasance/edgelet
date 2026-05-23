package run

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/ui"
)

func TestWriteHumanConfigResultSuccess(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	ctx := NewCLIContextWithWriters(&outBuf, &errBuf, ui.Options{NoColor: true}, output.FormatHuman)
	human := "config update: ok\naccepted:\n  - changeFrequencySeconds: 20 -> 10"
	if err := WriteHumanConfigResult(ctx, human, false); err != nil {
		t.Fatalf("WriteHumanConfigResult: %v", err)
	}
	if outBuf.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", outBuf.String())
	}
	stderr := errBuf.String()
	if !strings.Contains(stderr, "✔ config update: ok") {
		t.Fatalf("expected colored summary marker, got %q", stderr)
	}
	if !strings.Contains(stderr, "accepted:") || !strings.Contains(stderr, "20 -> 10") {
		t.Fatalf("expected plain detail lines, got %q", stderr)
	}
}

func TestWriteHumanConfigResultRejectionExit2(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	ctx := NewCLIContextWithWriters(&outBuf, &errBuf, ui.Options{NoColor: true}, output.FormatHuman)
	human := "config update: ok\naccepted:\n  - statusFrequencySeconds: 30 -> 10\nrejected:\n  - changeFrequencySeconds (bad value)"
	err := WriteHumanConfigResult(ctx, human, true)
	if err == nil {
		t.Fatal("expected error for partial rejection")
	}
	if ExitCodeForError(err) != ExitInvalidArgument {
		t.Fatalf("expected exit 2, got %d", ExitCodeForError(err))
	}
	stderr := errBuf.String()
	if !strings.Contains(stderr, "✘ config update: ok") {
		t.Fatalf("expected error summary marker, got %q", stderr)
	}
	if strings.Count(stderr, "✘") != 1 {
		t.Fatalf("expected single error marker, got %q", stderr)
	}
}

func TestWriteHumanMutationResultSingleLine(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	ctx := NewCLIContextWithWriters(&outBuf, &errBuf, ui.Options{NoColor: true}, output.FormatHuman)
	if err := WriteHumanMutationResult(ctx, "agent provisioned successfully (uuid: abc)"); err != nil {
		t.Fatalf("WriteHumanMutationResult: %v", err)
	}
	if outBuf.Len() != 0 {
		t.Fatalf("expected empty stdout for single-line mutation, got %q", outBuf.String())
	}
	stderr := errBuf.String()
	if !strings.Contains(stderr, "✔ agent provisioned successfully (uuid: abc)") {
		t.Fatalf("expected summary marker on stderr, got %q", stderr)
	}
}

func TestWriteHumanMutationResultMultiLine(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	ctx := NewCLIContextWithWriters(&outBuf, &errBuf, ui.Options{NoColor: true}, output.FormatHuman)
	human := "microservice start completed successfully (uuid=abc)\nwarning: controller reconcile may restart it"
	if err := WriteHumanMutationResult(ctx, human); err != nil {
		t.Fatalf("WriteHumanMutationResult: %v", err)
	}
	stderr := errBuf.String()
	if !strings.Contains(stderr, "✔ microservice start completed successfully (uuid=abc)") {
		t.Fatalf("expected summary marker on stderr, got %q", stderr)
	}
	if strings.Contains(stderr, "warning:") {
		t.Fatalf("expected detail off stderr, got %q", stderr)
	}
	stdout := outBuf.String()
	if !strings.Contains(stdout, "warning: controller reconcile may restart it") {
		t.Fatalf("expected detail on stdout, got %q", stdout)
	}
}
