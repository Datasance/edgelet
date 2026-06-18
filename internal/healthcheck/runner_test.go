package healthcheck

import (
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
)

func TestBuildHealthcheckCmd_CMDPassthrough(t *testing.T) {
	cmd := buildHealthcheckCmd(&models.Healthcheck{
		Test: []string{"CMD", "/bin/true", "--help"},
	})
	if len(cmd) != 2 || cmd[0] != "/bin/true" || cmd[1] != "--help" {
		t.Fatalf("CMD exec-form passthrough: got %v", cmd)
	}
}

func TestBuildHealthcheckCmd_CMDShellUsesShellFallback(t *testing.T) {
	script := "curl -f http://localhost/ || exit 1"
	cmd := buildHealthcheckCmd(&models.Healthcheck{
		Test: []string{"CMD-SHELL", script},
	})
	if len(cmd) != 3 || cmd[0] != "/bin/sh" || cmd[1] != "-lc" {
		t.Fatalf("CMD-SHELL wrapper prefix: got %v", cmd)
	}
	body := cmd[2]
	for _, want := range []string{
		"exec /bin/bash -c",
		"exec /bin/sh -c",
		"exec /busybox/sh -c",
		script,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("CMD-SHELL wrapper missing %q: %q", want, body)
		}
	}
}

func TestBuildHealthcheckCmd_InvalidReturnsNil(t *testing.T) {
	cases := []*models.Healthcheck{
		{Test: nil},
		{Test: []string{"CMD-SHELL"}},
		{Test: []string{"CMD"}},
	}
	for i, hc := range cases {
		if got := buildHealthcheckCmd(hc); got != nil {
			t.Fatalf("case %d: expected nil, got %v", i, got)
		}
	}
}

func TestParseHealthcheckFromLabels_UsesCanonicalKeyOnly(t *testing.T) {
	canonical := map[string]string{
		workloadmeta.LabelHealthcheck: `{"test":["CMD","echo ok"],"retries":2}`,
	}
	hc := parseHealthcheckFromLabels(canonical)
	if hc == nil {
		t.Fatal("expected healthcheck parsed from canonical label")
	}
	if len(hc.Test) != 2 || hc.Test[0] != "CMD" || hc.Test[1] != "echo ok" {
		t.Fatalf("unexpected parsed test command: %+v", hc.Test)
	}

	nonCanonicalOnly := map[string]string{
		"example.invalid/legacy-healthcheck": `{"test":["CMD","echo legacy"]}`,
	}
	if got := parseHealthcheckFromLabels(nonCanonicalOnly); got != nil {
		t.Fatal("expected nil when only non-canonical health label is present")
	}
}
