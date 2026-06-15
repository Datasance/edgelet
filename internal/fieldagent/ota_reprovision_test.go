package fieldagent

import (
	"testing"
)

func TestIsOTAInstallMethod(t *testing.T) {
	cases := map[string]bool{
		"upgrade":        true,
		"upgrade-airgap": true,
		"rollback":       true,
		"install":        false,
		"":               false,
	}
	for method, want := range cases {
		if got := isOTAInstallMethod(method); got != want {
			t.Fatalf("method=%q got=%v want=%v", method, got, want)
		}
	}
}
