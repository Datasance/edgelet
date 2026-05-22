package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseConfigSetArgs_DirectPairs(t *testing.T) {
	setMap, err := parseConfigSetArgs([]string{"networkInterface", "eth0", "-cf", "10", "secureMode", "true"})
	if err != nil {
		t.Fatalf("parseConfigSetArgs returned error: %v", err)
	}
	if got := setMap["networkInterface"]; got != "eth0" {
		t.Fatalf("expected networkInterface=eth0, got %v", got)
	}
	if got := setMap["changeFrequencySeconds"]; got != 10 {
		t.Fatalf("expected changeFrequencySeconds=10, got %v", got)
	}
	if got := setMap["secureMode"]; got != true {
		t.Fatalf("expected secureMode=true, got %v", got)
	}
}

func TestParseConfigSetArgs_RejectsUnknownKey(t *testing.T) {
	_, err := parseConfigSetArgs([]string{"unknownKey", "value"})
	if err == nil {
		t.Fatalf("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unsupported config key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigSetArgs_RejectsInvalidBool(t *testing.T) {
	_, err := parseConfigSetArgs([]string{"secureMode", "maybe"})
	if err == nil {
		t.Fatalf("expected error for invalid bool")
	}
	if !strings.Contains(err.Error(), "must be one of true|false|on|off|1|0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigSetArgs_ControllerCertRequiresReadablePEMPath(t *testing.T) {
	tempDir := t.TempDir()
	validCertPath := filepath.Join(tempDir, "controller.crt")
	if err := os.WriteFile(validCertPath, []byte(generateTestPEMCertificate(t)), 0o600); err != nil {
		t.Fatalf("failed to write test cert: %v", err)
	}

	setMap, err := parseConfigSetArgs([]string{"-ac", validCertPath})
	if err != nil {
		t.Fatalf("expected valid controller cert path, got error: %v", err)
	}
	if got := setMap["controllerCert"]; got != validCertPath {
		t.Fatalf("expected controllerCert path %q, got %v", validCertPath, got)
	}

	_, err = parseConfigSetArgs([]string{"-ac", filepath.Join(tempDir, "missing.crt")})
	if err == nil || !strings.Contains(err.Error(), "readable PEM certificate file path") {
		t.Fatalf("expected missing file validation error, got: %v", err)
	}
}

func generateTestPEMCertificate(t *testing.T) string {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "cli-test-cert",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestFormatConfigPatchResult_PrintsRejectedKeys(t *testing.T) {
	out := formatConfigPatchResult(map[string]interface{}{
		"status": "ok",
		"errorMap": map[string]interface{}{
			"a": "invalid",
		},
	})
	if !strings.Contains(out, "rejected keys") || !strings.Contains(out, "a: invalid") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFormatFlatMapWithOrder_PrefersConfiguredOrder(t *testing.T) {
	out := formatFlatMapWithOrder(map[string]interface{}{
		"controllerUrl":          "u",
		"connectionToController": "not provisioned",
		"cpuUsage":               "1%",
		"zzzExtra":               "x",
	}, statusOutputOrder)
	expectedPrefix := "connectionToController: not provisioned\ncpuUsage: 1%"
	if len(out) < len(expectedPrefix) || out[:len(expectedPrefix)] != expectedPrefix {
		t.Fatalf("unexpected order output: %s", out)
	}
}

func TestFormatFlatMapWithOrder_StatusIncludesAvailableNetworkInterfacesAfterTotalCPU(t *testing.T) {
	out := formatFlatMapWithOrder(map[string]interface{}{
		"systemTotalCpu":             "3200%",
		"availableNetworkInterfaces": "eth0, wlan0",
		"connectionToController":     "ok",
	}, statusOutputOrder)

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

func TestFormatFlatMapWithOrder_StatusIncludesAvailableRuntimesNearInterfaces(t *testing.T) {
	out := formatFlatMapWithOrder(map[string]interface{}{
		"systemTotalCpu":             "3200%",
		"availableNetworkInterfaces": "eth0, wlan0",
		"availableRuntimes":          "crun, edgelet",
		"connectionToController":     "ok",
	}, statusOutputOrder)

	interfacesLine := "availableNetworkInterfaces: eth0, wlan0"
	runtimesLine := "availableRuntimes: crun, edgelet"
	interfacesIdx := strings.Index(out, interfacesLine)
	runtimesIdx := strings.Index(out, runtimesLine)
	if interfacesIdx == -1 || runtimesIdx == -1 {
		t.Fatalf("expected available interface/runtime lines in output, got: %s", out)
	}
	if runtimesIdx < interfacesIdx {
		t.Fatalf("expected availableRuntimes after availableNetworkInterfaces, got: %s", out)
	}
}

func TestFormatInfoWithAliasOrder_UsesAliasesAndOrder(t *testing.T) {
	out := formatInfoWithAliasOrder(map[string]interface{}{
		"iofogUuid":                   "abc",
		"namespace":                   "default",
		"networkInterface":            "eth0",
		"ipAddress":                   "1.2.3.4",
		"controllerUrl":               "http://localhost",
		"controllerCert":              "/tmp/cert.crt",
		"secureMode":                  "off",
		"containerEngine":             "docker",
		"dockerUrl":                   "unix:///var/run/docker.sock",
		"fogType":                     "auto",
		"availableDiskThreshold":      "20",
		"changeUpdateFrequency":       "10",
		"statusUpdateFrequency":       "10",
		"cpuUsageLimit":               "80.00%",
		"memoryRamLimit":              "4096.00 MiB",
		"diskUsageLimit":              "10.00 GiB",
		"diskDirectory":               "/var/lib/iofog-agent",
		"logDiskLimit":                "10.00 GiB",
		"logFileDirectory":            "/var/log/iofog-agent",
		"logFilesCount":               "10",
		"logFilesLevel":               "DEBUG",
		"readyToUpgradeScanFrequency": "24",
		"scanDevicesFrequency":        "60",
		"dockerPruningFrequency":      "0",
		"edgeGuardFrequency":          "0",
		"gpsCoordinates":              "0,0",
		"gpsDevice":                   "/dev/ttyUSB0",
		"gpsScanFrequency":            "60",
		"gpsMode":                     "auto",
		"watchdogEnabled":             "on",
		"developerMode":               "off",
		"timeZone":                    "Etc/UTC",
	})

	if !strings.Contains(out, "arch: auto") {
		t.Fatalf("expected fogType to be rendered as arch, got: %s", out)
	}
	if !strings.Contains(out, "cpuLimit: 80.00%") {
		t.Fatalf("expected cpuUsageLimit to be rendered as cpuLimit, got: %s", out)
	}
	if !strings.Contains(out, "logLevel: DEBUG") {
		t.Fatalf("expected logFilesLevel to be rendered as logLevel, got: %s", out)
	}
	if strings.Contains(out, "fogType: ") || strings.Contains(out, "cpuUsageLimit: ") || strings.Contains(out, "logFilesLevel: ") {
		t.Fatalf("unexpected canonical keys leaked into output: %s", out)
	}
}

func TestFormatProvisionSuccess(t *testing.T) {
	withUUID := formatProvisionSuccess("abc-123")
	if withUUID != "agent provisioned successfully (uuid: abc-123)" {
		t.Fatalf("unexpected provision message with UUID: %s", withUUID)
	}

	withoutUUID := formatProvisionSuccess("<unknown>")
	if withoutUUID != "agent provisioned successfully" {
		t.Fatalf("unexpected provision message without UUID: %s", withoutUUID)
	}
}

func TestFormatVersionOutput_DaemonUnavailableFallback(t *testing.T) {
	out := formatVersionOutput("v1.2.3", "2026-01-01", "abcdef0", nil, errors.New("dial failure"))
	if !strings.Contains(out, "cli.version: v1.2.3") {
		t.Fatalf("expected cli version output, got: %s", out)
	}
	if !strings.Contains(out, "daemon: unavailable") {
		t.Fatalf("expected daemon unavailable fallback, got: %s", out)
	}
}

func TestFormatVersionOutput_IncludesDaemonFields(t *testing.T) {
	out := formatVersionOutput("v1.2.3", "2026-01-01", "abcdef0", map[string]interface{}{
		"version":                "v9.9.9",
		"buildTime":              "2026-05-13",
		"gitCommit":              "deadbee",
		"flavor":                 "lite",
		"allowedContainerEngine": "docker,podman",
	}, nil)
	if !strings.Contains(out, "daemon.version: v9.9.9") ||
		!strings.Contains(out, "daemon.buildTime: 2026-05-13") ||
		!strings.Contains(out, "daemon.gitCommit: deadbee") ||
		!strings.Contains(out, "daemon.flavor: lite") ||
		!strings.Contains(out, "daemon.allowedContainerEngine: docker,podman") {
		t.Fatalf("unexpected daemon composite version output: %s", out)
	}
}

func TestFormatV3Output_MSListHandlesQueryPath(t *testing.T) {
	out := formatV3Output("/v3/ms?source=all", map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"uuid":        "u1",
				"application": "app",
				"name":        "ms",
				"source":      "local",
				"state":       "running",
				"containerId": "c1",
				"image":       "img:1",
				"type":        "local",
			},
		},
	})
	if !strings.Contains(out, "UUID") || !strings.Contains(out, "APPLICATIONNAME") {
		t.Fatalf("expected ms table headers, got: %s", out)
	}
	if strings.Contains(out, "SOURCE") {
		t.Fatalf("did not expect SOURCE column in ms table output, got: %s", out)
	}
	if !strings.Contains(out, "u1") {
		t.Fatalf("expected row content, got: %s", out)
	}
}

func TestFormatV3Output_MSLifecycleFormatting(t *testing.T) {
	out := formatV3Output("/v3/ms/abc/start", map[string]interface{}{
		"status":           "ok",
		"microserviceUuid": "abc",
		"warning":          "controller reconcile may restart it",
	})
	if !strings.Contains(out, "microservice start completed successfully") {
		t.Fatalf("expected lifecycle success message, got: %s", out)
	}
	if !strings.Contains(out, "warning: controller reconcile may restart it") {
		t.Fatalf("expected warning visibility, got: %s", out)
	}
}

func TestFormatRegistryInspect_HumanReadable(t *testing.T) {
	out := formatRegistryInspect(map[string]interface{}{
		"id":        3,
		"url":       "registry.example.com",
		"isPublic":  false,
		"userName":  "john",
		"userEmail": "john@example.com",
		"password":  "s3cr3t",
	}, false)
	if strings.Contains(out, "{") || strings.Contains(out, "}") {
		t.Fatalf("expected non-JSON inspect output, got: %s", out)
	}
	if !strings.Contains(out, "ID: 3") || !strings.Contains(out, "URL: registry.example.com") {
		t.Fatalf("expected inspect fields in output, got: %s", out)
	}
	expectedB64 := base64.StdEncoding.EncodeToString([]byte("s3cr3t"))
	if !strings.Contains(out, "PASSWORD_B64: "+expectedB64) {
		t.Fatalf("expected PASSWORD_B64 output, got: %s", out)
	}
	if strings.Contains(out, "PASSWORD: s3cr3t") {
		t.Fatalf("did not expect plain password by default, got: %s", out)
	}
}

func TestFormatRegistryInspect_PlainPasswordOptIn(t *testing.T) {
	out := formatRegistryInspect(map[string]interface{}{
		"id":       3,
		"url":      "registry.example.com",
		"isPublic": false,
		"userName": "john",
		"password": "s3cr3t",
	}, true)
	if !strings.Contains(out, "PASSWORD: s3cr3t") {
		t.Fatalf("expected plain password output, got: %s", out)
	}
	if strings.Contains(out, "PASSWORD_B64:") {
		t.Fatalf("did not expect PASSWORD_B64 in plain mode, got: %s", out)
	}
}

func TestFormatRegistryInspect_PublicRegistryOmitsPassword(t *testing.T) {
	out := formatRegistryInspect(map[string]interface{}{
		"id":       1,
		"url":      "docker.io",
		"isPublic": true,
	}, false)
	if strings.Contains(out, "PASSWORD") {
		t.Fatalf("expected no password fields for public registry, got: %s", out)
	}
}

func TestFormatLogEntries_PreservesDockerStyleSpacing(t *testing.T) {
	out := formatLogEntries(map[string]interface{}{
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

func TestFormatLogEntries_AppendsMissingTrailingNewlinePerEntry(t *testing.T) {
	out := formatLogEntries(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"line": "line1"},
			map[string]interface{}{"line": ""},
			map[string]interface{}{"line": "line3"},
		},
	}, false)
	if out != "line1\n\nline3\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFormatDeployApplyResult_KindSpecific(t *testing.T) {
	msOut := formatDeployApplyResult(map[string]interface{}{
		"accepted":     true,
		"kind":         "Microservice",
		"deploymentId": "dep-1",
	})
	if !strings.Contains(msOut, "microservice manifest applied successfully") || !strings.Contains(msOut, "dep-1") {
		t.Fatalf("expected microservice apply output, got: %s", msOut)
	}

	regOut := formatDeployApplyResult(map[string]interface{}{
		"accepted": true,
		"kind":     "Registry",
		"registry": map[string]interface{}{
			"id":  7,
			"url": "docker.io",
		},
	})
	if !strings.Contains(regOut, "registry manifest applied successfully") || !strings.Contains(regOut, "id=7") {
		t.Fatalf("expected registry apply output, got: %s", regOut)
	}

	rcOut := formatDeployApplyResult(map[string]interface{}{
		"accepted": true,
		"kind":     "RuntimeClass",
		"runtimeClass": map[string]interface{}{
			"name":    "edgelet",
			"handler": "edgelet",
		},
	})
	if !strings.Contains(rcOut, "runtimeclass manifest applied successfully") || !strings.Contains(rcOut, "name=edgelet") {
		t.Fatalf("expected runtimeclass apply output, got: %s", rcOut)
	}
}

func TestFormatDeployApplyResult_AsyncSucceeded(t *testing.T) {
	out := formatDeployApplyResult(map[string]interface{}{
		"status":       "succeeded",
		"deploymentId": "dep-42",
	})
	if !strings.Contains(out, "microservice manifest applied successfully") || !strings.Contains(out, "dep-42") {
		t.Fatalf("expected async deploy success output, got: %s", out)
	}
}

func TestFormatDeployApplyResult_RuntimeClassOperationRunning(t *testing.T) {
	out := formatDeployApplyResult(map[string]interface{}{
		"status":      "running",
		"kind":        "RuntimeClass",
		"operationId": "op-123",
		"stage":       "reconfiguring",
	})
	if !strings.Contains(out, "runtimeclass apply is still in progress") ||
		!strings.Contains(out, "operationId=op-123") ||
		!strings.Contains(out, "/v3/deploy/runtimeclasses:apply/op-123") {
		t.Fatalf("expected runtimeclass operation in-progress output, got: %s", out)
	}
}

func TestFormatDeployApplyProgressLine(t *testing.T) {
	if got := formatDeployApplyProgressLine("pulling"); !strings.Contains(got, "(pulling)") {
		t.Fatalf("expected stage in progress line, got: %s", got)
	}
	if got := formatDeployApplyProgressLine(""); got != "applying microservice manifest..." {
		t.Fatalf("unexpected fallback progress line: %s", got)
	}
}

func TestFormatDeployApplyError(t *testing.T) {
	code, message := formatDeployApplyError(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    "INTERNAL",
			"message": "failed to start",
		},
	})
	if code != "INTERNAL" || message != "failed to start" {
		t.Fatalf("unexpected structured error mapping: code=%s message=%s", code, message)
	}
	code, message = formatDeployApplyError(map[string]interface{}{})
	if code != "INTERNAL" || message != "deploy apply failed" {
		t.Fatalf("unexpected fallback error mapping: code=%s message=%s", code, message)
	}
}

func TestHandleRuntimeClassApplyWithProgress_PollSucceeded(t *testing.T) {
	prevStart := runtimeClassApplyStartRequest
	prevStatus := runtimeClassApplyStatusRequest
	prevTimeout := runtimeClassApplyPollTimeout
	prevInterval := runtimeClassApplyPollInterval
	t.Cleanup(func() {
		runtimeClassApplyStartRequest = prevStart
		runtimeClassApplyStatusRequest = prevStatus
		runtimeClassApplyPollTimeout = prevTimeout
		runtimeClassApplyPollInterval = prevInterval
	})

	runtimeClassApplyPollTimeout = 2 * time.Second
	runtimeClassApplyPollInterval = 1 * time.Millisecond
	runtimeClassApplyStartRequest = func(_ *Client, _ string, _ map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"status":      "running",
			"kind":        "RuntimeClass",
			"operationId": "op-1",
			"stage":       "persisting",
		}, nil
	}
	polls := 0
	runtimeClassApplyStatusRequest = func(_ *Client, _ string) (map[string]interface{}, error) {
		polls++
		if polls < 2 {
			return map[string]interface{}{"status": "running", "stage": "reconfiguring"}, nil
		}
		return map[string]interface{}{
			"status": "succeeded",
			"runtimeClass": map[string]interface{}{
				"name":    "edgelet",
				"handler": "edgelet-wasm",
			},
		}, nil
	}

	out := handleRuntimeClassApplyWithProgress(&Client{}, "/tmp/runtimeclass.yaml", map[string]string{"async": "true"})
	if !strings.Contains(out, "runtimeclass manifest applied successfully") || !strings.Contains(out, "name=edgelet") {
		t.Fatalf("expected runtimeclass apply success output, got: %s", out)
	}
}

func TestHandleRuntimeClassApplyWithProgress_PollFailed(t *testing.T) {
	prevStart := runtimeClassApplyStartRequest
	prevStatus := runtimeClassApplyStatusRequest
	prevTimeout := runtimeClassApplyPollTimeout
	prevInterval := runtimeClassApplyPollInterval
	t.Cleanup(func() {
		runtimeClassApplyStartRequest = prevStart
		runtimeClassApplyStatusRequest = prevStatus
		runtimeClassApplyPollTimeout = prevTimeout
		runtimeClassApplyPollInterval = prevInterval
	})

	runtimeClassApplyPollTimeout = 2 * time.Second
	runtimeClassApplyPollInterval = 1 * time.Millisecond
	runtimeClassApplyStartRequest = func(_ *Client, _ string, _ map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"status":      "running",
			"kind":        "RuntimeClass",
			"operationId": "op-2",
			"stage":       "persisting",
		}, nil
	}
	runtimeClassApplyStatusRequest = func(_ *Client, _ string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"status": "failed",
			"error": map[string]interface{}{
				"code":    "INVALID_ARGUMENT",
				"message": "invalid runtimeclass manifest",
			},
		}, nil
	}

	out := handleRuntimeClassApplyWithProgress(&Client{}, "/tmp/runtimeclass.yaml", map[string]string{"async": "true"})
	expected := "Error[INVALID_ARGUMENT]: invalid runtimeclass manifest"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestHandleRuntimeClassApplyWithProgress_TimeoutFallback(t *testing.T) {
	prevStart := runtimeClassApplyStartRequest
	prevStatus := runtimeClassApplyStatusRequest
	prevTimeout := runtimeClassApplyPollTimeout
	prevInterval := runtimeClassApplyPollInterval
	t.Cleanup(func() {
		runtimeClassApplyStartRequest = prevStart
		runtimeClassApplyStatusRequest = prevStatus
		runtimeClassApplyPollTimeout = prevTimeout
		runtimeClassApplyPollInterval = prevInterval
	})

	runtimeClassApplyPollTimeout = 5 * time.Millisecond
	runtimeClassApplyPollInterval = 1 * time.Millisecond
	runtimeClassApplyStartRequest = func(_ *Client, _ string, _ map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"status":      "running",
			"kind":        "RuntimeClass",
			"operationId": "op-3",
			"stage":       "persisting",
		}, nil
	}
	runtimeClassApplyStatusRequest = func(_ *Client, _ string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"status": "running",
			"stage":  "reconfiguring",
		}, nil
	}

	out := handleRuntimeClassApplyWithProgress(&Client{}, "/tmp/runtimeclass.yaml", map[string]string{"async": "true"})
	if !strings.Contains(out, "runtimeclass apply is still in progress") ||
		!strings.Contains(out, "operationId=op-3") ||
		!strings.Contains(out, "/v3/deploy/runtimeclasses:apply/op-3") {
		t.Fatalf("expected timeout fallback output with operation details, got: %s", out)
	}
}

func TestFormatV3Output_ImageListTable(t *testing.T) {
	out := formatV3Output("/v3/images", map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"repository": "ghcr.io/datasance/nats",
				"tag":        "2.12.4",
				"shortId":    "5af03c2768d3",
				"createdAt":  "2026-05-13T14:21:30Z",
				"sizeHuman":  "52.0 MB",
			},
		},
	})
	if !strings.Contains(out, "REPOSITORY") || !strings.Contains(out, "SIZE") {
		t.Fatalf("expected image table headers, got: %s", out)
	}
	if !strings.Contains(out, "ghcr.io/datasance/nats") || !strings.Contains(out, "52.0 MB") {
		t.Fatalf("expected image row, got: %s", out)
	}
}

func TestFormatV3Output_ImagePruneMessage(t *testing.T) {
	out := formatV3Output("/v3/images:prune", map[string]interface{}{
		"deletedCount":        2,
		"spaceReclaimedHuman": "8.0 MB",
		"engine":              "docker",
	})
	if !strings.Contains(out, "pruned dangling images") ||
		!strings.Contains(out, "deleted=2") ||
		!strings.Contains(out, "reclaimed=8.0 MB") {
		t.Fatalf("unexpected prune output: %s", out)
	}
}

func TestHandleImageV3UsageValidation(t *testing.T) {
	client := &Client{}
	if got := handleImageV3(client, []string{"pull"}); !strings.Contains(got, "Usage: iofog-agent image pull") {
		t.Fatalf("expected pull usage, got: %s", got)
	}
	if got := handleImageV3(client, []string{"load"}); !strings.Contains(got, "Usage: iofog-agent image load -f") {
		t.Fatalf("expected load usage, got: %s", got)
	}
	if got := handleImageV3(client, []string{"prune", "invalid"}); !strings.Contains(got, "Error[INVALID_ARGUMENT]") {
		t.Fatalf("expected prune invalid argument, got: %s", got)
	}
	if got := handleImageV3(client, []string{"rm"}); !strings.Contains(got, "Usage: iofog-agent image rm") {
		t.Fatalf("expected rm usage, got: %s", got)
	}
}

func TestFormatV3Output_ImageRemoveMessage(t *testing.T) {
	out := formatV3Output("/v3/images:remove", map[string]interface{}{
		"removed": "sha256:abc123",
		"engine":  "docker",
	})
	if !strings.Contains(out, "image removed successfully") || !strings.Contains(out, "sha256:abc123") {
		t.Fatalf("unexpected image remove output: %s", out)
	}
}

func TestFormatV3Output_RuntimeClassListAndInspect(t *testing.T) {
	listOut := formatV3Output("/v3/deploy/runtimeclasses", map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"name":        "edgelet",
				"handler":     "edgelet",
				"runtimeName": "edgelet",
			},
		},
	})
	if !strings.Contains(listOut, "NAME") || !strings.Contains(listOut, "edgelet") {
		t.Fatalf("expected runtimeclass list table, got: %s", listOut)
	}

	inspectOut := formatV3Output("/v3/deploy/runtimeclasses/edgelet", map[string]interface{}{
		"name":        "edgelet",
		"handler":     "edgelet",
		"runtimeName": "edgelet",
	})
	if !strings.Contains(inspectOut, "NAME: edgelet") || !strings.Contains(inspectOut, "RUNTIME: edgelet") {
		t.Fatalf("expected runtimeclass inspect output, got: %s", inspectOut)
	}
}

func TestFormatV3Output_RuntimeClassRemoveMessage(t *testing.T) {
	out := formatV3Output("/v3/deploy/runtimeclasses/edgelet", map[string]interface{}{
		"status": "ok",
		"name":   "edgelet",
	})
	if !strings.Contains(out, "runtimeclass removed successfully") || !strings.Contains(out, "name=edgelet") {
		t.Fatalf("unexpected runtimeclass remove output: %s", out)
	}
}

func TestFormatV3RequestError_RuntimeClassUnsupportedMessage(t *testing.T) {
	err := &V3APIError{
		StatusCode: 400,
		Code:       "INVALID_ARGUMENT",
		Message:    "runtimeclass is supported only when containerEngine=iofog on full flavor builds",
	}
	out := formatV3RequestError(err)
	expected := "Error[INVALID_ARGUMENT]: runtimeclass is supported only when containerEngine=iofog on full flavor builds"
	if out != expected {
		t.Fatalf("unexpected formatted error: got=%q want=%q", out, expected)
	}
}
