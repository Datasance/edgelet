package output

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/cli/ui"
)

func TestFormatConfigPatchResult_PrintsRejectedKeys(t *testing.T) {
	out := FormatConfigPatchResult(map[string]interface{}{
		"status": "ok",
		"errorMap": map[string]interface{}{
			"a": "invalid",
		},
	})
	if !strings.Contains(out, "rejected keys") || !strings.Contains(out, "a: invalid") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFormatEdgeletAPIHuman_StatusOrder(t *testing.T) {
	out := FormatEdgeletAPIHuman("/v1/system/status", map[string]interface{}{
		"controllerUrl":          "u",
		"connectionToController": "not provisioned",
		"cpuUsage":               "1%",
		"zzzExtra":               "x",
	})
	expectedPrefix := "connectionToController: not provisioned\ncpuUsage: 1%"
	if len(out) < len(expectedPrefix) || out[:len(expectedPrefix)] != expectedPrefix {
		t.Fatalf("unexpected order output: %s", out)
	}
}

func TestFormatEdgeletAPIHuman_StatusIncludesAvailableNetworkInterfacesAfterTotalCPU(t *testing.T) {
	out := FormatEdgeletAPIHuman("/v1/system/status", map[string]interface{}{
		"systemTotalCpu":             "3200%",
		"availableNetworkInterfaces": "eth0, wlan0",
		"connectionToController":     "ok",
	})
	totalCPULine := "systemTotalCpu: 3200%"
	availableInterfacesLine := "availableNetworkInterfaces: eth0, wlan0"
	totalIdx := strings.Index(out, totalCPULine)
	availableIdx := strings.Index(out, availableInterfacesLine)
	if totalIdx == -1 || availableIdx == -1 {
		t.Fatalf("expected both status lines in output, got: %s", out)
	}
	if availableIdx < totalIdx {
		t.Fatalf("expected available interfaces after systemTotalCpu, got: %s", out)
	}
}

func TestFormatProvisionSuccess(t *testing.T) {
	withUUID := FormatProvisionSuccess("abc-123")
	if withUUID != "agent provisioned successfully (uuid: abc-123)" {
		t.Fatalf("unexpected provision message with UUID: %s", withUUID)
	}
	withoutUUID := FormatProvisionSuccess("<unknown>")
	if withoutUUID != "agent provisioned successfully" {
		t.Fatalf("unexpected provision message without UUID: %s", withoutUUID)
	}
}

func TestFormatVersionHuman_DaemonUnavailableFallback(t *testing.T) {
	out := FormatVersionHuman("v1.2.3", "2026-01-01", "abcdef0", nil, errors.New("dial failure"))
	if !strings.Contains(out, "cli.version: v1.2.3") {
		t.Fatalf("expected cli version output, got: %s", out)
	}
	if !strings.Contains(out, "daemon: unavailable") {
		t.Fatalf("expected daemon unavailable fallback, got: %s", out)
	}
}

func TestFormatEdgeletAPIHuman_MSListHandlesQueryPath(t *testing.T) {
	out := FormatEdgeletAPIHuman("/v1/ms?source=all", map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"uuid":        "u1",
				"application": "app",
				"name":        "ms",
				"state":       "running",
				"containerId": "c1",
				"image":       "img:1",
				"type":        "local",
			},
		},
	})
	if !strings.Contains(out, "UUID") || !strings.Contains(out, "u1") {
		t.Fatalf("expected ms table output, got: %s", out)
	}
}

func TestFormatEdgeletAPIHuman_MSLifecycleFormatting(t *testing.T) {
	out := FormatEdgeletAPIHuman("/v1/ms/abc/start", map[string]interface{}{
		"status":           "ok",
		"microserviceUuid": "abc",
		"warning":          "controller reconcile may restart it",
	})
	if !strings.Contains(out, "microservice start completed successfully") {
		t.Fatalf("expected lifecycle success message, got: %s", out)
	}
}

func TestFormatRegistryInspect_HumanReadable(t *testing.T) {
	out := FormatRegistryInspect(map[string]interface{}{
		"id": 3, "url": "registry.example.com", "isPublic": false,
		"userName": "john", "userEmail": "john@example.com", "password": "s3cr3t",
	}, false)
	expectedB64 := base64.StdEncoding.EncodeToString([]byte("s3cr3t"))
	if !strings.Contains(out, "PASSWORD_B64: "+expectedB64) {
		t.Fatalf("expected PASSWORD_B64 output, got: %s", out)
	}
}

func TestFormatLogEntries_PreservesDockerStyleSpacing(t *testing.T) {
	out := FormatLogEntries(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"line": "line1\n"},
			map[string]interface{}{"line": "\n"},
			map[string]interface{}{"line": "line3\n"},
		},
	}, false)
	if out != "line1\n\nline3\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFormatDeployApplyResult_KindSpecific(t *testing.T) {
	msOut := formatDeployApplyResult(map[string]interface{}{
		"accepted": true, "kind": "Microservice", "deploymentId": "dep-1",
	})
	if !strings.Contains(msOut, "microservice manifest applied successfully") {
		t.Fatalf("expected microservice apply output, got: %s", msOut)
	}
}

func TestFormatDeployStageLine(t *testing.T) {
	if got := ui.FormatDeployStageLine("pulling"); !strings.Contains(got, "(pulling)") {
		t.Fatalf("expected stage in progress line, got: %s", got)
	}
}
