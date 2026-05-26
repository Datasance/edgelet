package run

import (
	"bytes"
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/ui"
)

func TestWithSpinnerHumanModeShowsMessage(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	ctx := NewCLIContextWithWriters(&outBuf, &errBuf, ui.Options{NoColor: true}, output.FormatHuman)

	called := false
	err := WithSpinner(ctx, "Working...", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithSpinner: %v", err)
	}
	if !called {
		t.Fatal("expected fn to run")
	}
	if outBuf.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", outBuf.String())
	}
	if !strings.Contains(errBuf.String(), "Working...") {
		t.Fatalf("expected spinner message on stderr, got %q", errBuf.String())
	}
}

func TestWithSpinnerStructuredSkipsSpinner(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	ctx := NewCLIContextWithWriters(&outBuf, &errBuf, ui.Options{NoColor: true}, output.FormatJSON)

	err := WithSpinner(ctx, "Working...", func() error { return nil })
	if err != nil {
		t.Fatalf("WithSpinner: %v", err)
	}
	if strings.TrimSpace(errBuf.String()) != "" {
		t.Fatalf("expected no stderr UX for structured output, got %q", errBuf.String())
	}
}

func TestWithSpinnerHumanSuccessWritesCheckmark(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	ctx := NewCLIContextWithWriters(&outBuf, &errBuf, ui.Options{NoColor: true}, output.FormatHuman)

	err := WithSpinnerHumanSuccess(ctx, "Pulling image...", func() (string, error) {
		return "image pulled successfully", nil
	})
	if err != nil {
		t.Fatalf("WithSpinnerHumanSuccess: %v", err)
	}
	if outBuf.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", outBuf.String())
	}
	stderr := errBuf.String()
	if !strings.Contains(stderr, "Pulling image...") || !strings.Contains(stderr, "✔ image pulled successfully") {
		t.Fatalf("expected spinner line and success marker on stderr, got %q", stderr)
	}
}
