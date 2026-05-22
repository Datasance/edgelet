package system

import (
	"strings"
	"testing"
)

func TestParseLogsOptions_RejectsUnknownFlag(t *testing.T) {
	_, err := ParseLogsOptions([]string{"--bad"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected invalid argument error, got: %v", err)
	}
}

func TestParseLogsOptions_MissingTailValue(t *testing.T) {
	_, err := ParseLogsOptions([]string{"--tail"})
	if err == nil || !strings.Contains(err.Error(), "--tail requires a number") {
		t.Fatalf("expected missing tail value error, got: %v", err)
	}
}
