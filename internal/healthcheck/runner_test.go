package healthcheck

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
)

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
