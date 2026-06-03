package output

import (
	"strings"
	"testing"
)

func TestFormatEdgeletAPIHuman_ControlPlaneStatus(t *testing.T) {
	out := FormatEdgeletAPIHuman("/v1/system/controlplane", map[string]interface{}{
		"controllerUuid": "uuid-1",
		"namespace":      "default",
		"name":           "pot",
		"state":          "running",
	})
	for _, part := range []string{"controllerUuid: uuid-1", "namespace: default", "name: pot"} {
		if !strings.Contains(out, part) {
			t.Fatalf("expected %q in output: %q", part, out)
		}
	}
}

func TestFormatEdgeletAPIHuman_ControlPlaneDelete(t *testing.T) {
	out := FormatEdgeletAPIHuman("/v1/system/controlplane", map[string]interface{}{"status": "ok"})
	if out != "control plane deployment removed successfully" {
		t.Fatalf("unexpected delete output: %q", out)
	}
}

func TestFormatEdgeletAPIHuman_ControlPlaneManifest(t *testing.T) {
	out := FormatEdgeletAPIHuman("/v1/system/controlplane/manifest", map[string]interface{}{
		"manifestYaml": "kind: ControlPlane",
		"masked":       true,
	})
	for _, part := range []string{"secrets masked", "kind: ControlPlane"} {
		if !strings.Contains(out, part) {
			t.Fatalf("expected %q in output: %q", part, out)
		}
	}
}
