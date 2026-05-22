package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestShowMSHelpV3_IncludesLocalDottedSelector(t *testing.T) {
	help := showMSHelpV3()
	if !strings.Contains(help, "local.<name>") {
		t.Fatalf("expected help to document local.<name> selector, got: %q", help)
	}
}

func TestHandleDeprovisionV3_RejectsInvalidScopeFlag(t *testing.T) {
	out := handleDeprovisionV3(&Client{}, []string{"--scope", "bad"})
	if !strings.Contains(out, "Error[INVALID_ARGUMENT]") {
		t.Fatalf("expected invalid argument error, got: %s", out)
	}
}

func TestHandleDeprovisionV3_RejectsUnknownFlag(t *testing.T) {
	out := handleDeprovisionV3(&Client{}, []string{"--bad-flag"})
	if !strings.Contains(out, "Error[INVALID_ARGUMENT]") {
		t.Fatalf("expected invalid argument error, got: %s", out)
	}
}

func TestHandlePruneV3_RejectsInvalidMode(t *testing.T) {
	out := handlePruneV3(&Client{}, []string{"invalid"})
	if !strings.Contains(out, "Error[INVALID_ARGUMENT]") {
		t.Fatalf("expected invalid argument error, got: %s", out)
	}
}

func TestHandleSystemV3_PruneRejectsInvalidMode(t *testing.T) {
	out := handleSystemV3(&Client{}, []string{"prune", "invalid"})
	if !strings.Contains(out, "Error[INVALID_ARGUMENT]") {
		t.Fatalf("expected invalid argument error, got: %s", out)
	}
}

func TestHandleImageV3_PruneRejectsInvalidMode(t *testing.T) {
	out := handleImageV3(&Client{}, []string{"prune", "invalid"})
	if !strings.Contains(out, "Error[INVALID_ARGUMENT]") {
		t.Fatalf("expected invalid argument error, got: %s", out)
	}
}

func TestHandleImageV3_PruneRejectsNonDanglingMode(t *testing.T) {
	out := handleImageV3(&Client{}, []string{"prune", "volumes"})
	if !strings.Contains(out, "supports only dangling mode") {
		t.Fatalf("expected dangling-only validation error, got: %s", out)
	}
}

func TestHandleSystemV3_LogsRejectsUnknownFlag(t *testing.T) {
	out := handleSystemV3(&Client{}, []string{"logs", "--bad"})
	if !strings.Contains(out, "Error[INVALID_ARGUMENT]") {
		t.Fatalf("expected invalid argument error, got: %s", out)
	}
}

func TestHandleSystemV3_LogsMissingTailValue(t *testing.T) {
	out := handleSystemV3(&Client{}, []string{"logs", "--tail"})
	if !strings.Contains(out, "--tail requires a number") {
		t.Fatalf("expected missing tail value error, got: %s", out)
	}
}

func TestParseRegistryInspectArgs_PasswordPlain(t *testing.T) {
	id, passwordPlain, err := parseRegistryInspectArgs([]string{"7", "--password-plain"})
	if err != "" {
		t.Fatalf("expected no parse error, got: %s", err)
	}
	if id != "7" {
		t.Fatalf("expected id 7, got: %s", id)
	}
	if !passwordPlain {
		t.Fatalf("expected passwordPlain=true")
	}
}

func TestParseRegistryInspectArgs_RejectsUnknownFlag(t *testing.T) {
	_, _, err := parseRegistryInspectArgs([]string{"7", "--bad"})
	if !strings.Contains(err, "Error[INVALID_ARGUMENT]") {
		t.Fatalf("expected invalid argument error, got: %s", err)
	}
}

func TestHandleRuntimeClassV3_UsageValidation(t *testing.T) {
	client := &Client{}
	if got := handleRuntimeClassV3(client, []string{"inspect"}); !strings.Contains(got, "Usage: iofog-agent runtimeclass inspect") {
		t.Fatalf("expected inspect usage, got: %s", got)
	}
	if got := handleRuntimeClassV3(client, []string{"rm"}); !strings.Contains(got, "Usage: iofog-agent runtimeclass rm") {
		t.Fatalf("expected rm usage, got: %s", got)
	}
}

func TestShowDeployHelpV3_IncludesRuntimeClassFlow(t *testing.T) {
	help := showDeployHelpV3()
	if !strings.Contains(help, "deploy runtimeclass apply") || !strings.Contains(help, "deploy runtimeclass validate") {
		t.Fatalf("expected runtimeclass deploy flow in help, got: %s", help)
	}
}

func TestWriteStreamLogLine_PreservesIncomingNewline(t *testing.T) {
	var b bytes.Buffer
	writeStreamLogLine(&b, "", "line1\n\nline3\n", false)
	if b.String() != "line1\n\nline3\n" {
		t.Fatalf("unexpected output: %q", b.String())
	}
}

func TestWriteStreamLogLine_AppendsMissingTrailingNewline(t *testing.T) {
	var b bytes.Buffer
	writeStreamLogLine(&b, "", "line-without-newline", false)
	if b.String() != "line-without-newline\n" {
		t.Fatalf("unexpected output: %q", b.String())
	}
}
