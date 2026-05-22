package registry

import (
	"strings"
	"testing"
)

func TestParseInspectArgs_PasswordPlain(t *testing.T) {
	parsed, err := ParseInspectArgs([]string{"7", "--password-plain"})
	if err != nil {
		t.Fatalf("expected no parse error, got: %v", err)
	}
	if parsed.ID != "7" || !parsed.PasswordPlain {
		t.Fatalf("unexpected parse result: %+v", parsed)
	}
}

func TestParseInspectArgs_RejectsUnknownFlag(t *testing.T) {
	_, err := ParseInspectArgs([]string{"7", "--bad"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected invalid argument error, got: %v", err)
	}
}
