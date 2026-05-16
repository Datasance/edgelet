package cli

import (
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
