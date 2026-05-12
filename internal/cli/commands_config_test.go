package cli

import (
	"errors"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
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
		"controllerUrl":         "u",
		"connectionToController": "not provisioned",
		"cpuUsage":               "1%",
		"zzzExtra":               "x",
	}, statusOutputOrder)
	expectedPrefix := "connectionToController: not provisioned\ncpuUsage: 1%"
	if len(out) < len(expectedPrefix) || out[:len(expectedPrefix)] != expectedPrefix {
		t.Fatalf("unexpected order output: %s", out)
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
		"postDiagnosticsFrequency":    "10",
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

