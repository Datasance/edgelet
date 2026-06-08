package version

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestParseVersionCommand(t *testing.T) {
	cmd, err := ParseVersionCommand(map[string]any{"command": "upgrade"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != VersionCommandUpgrade {
		t.Fatalf("expected UPGRADE, got %q", cmd)
	}

	if _, err := ParseVersionCommand(map[string]any{"command": "invalid"}); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestIsReadyToUpgrade_RequiresInstallScriptAndHealthyDaemon(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "install.sh")
	receipt := filepath.Join(dir, "install-receipt")

	writeFile(t, script, "#!/bin/sh\n")
	writeKV(t, receipt, map[string]string{
		"installed_version": "1.0.0",
		"os":                "linux",
		"arch":              "amd64",
	})

	rm := NewReleaseManager(
		WithPaths(script, receipt, filepath.Join(dir, "previous-release"), filepath.Join(dir, "cache")),
		WithRunningVersion(func() string { return "1.0.0" }),
	)
	h := NewHandler(rm)
	h.isContainer = func() bool { return false }
	h.isDaemonHealthy = func() bool { return false }

	if h.IsReadyToUpgradeWithAction(map[string]any{"version": "2.0.0"}) {
		t.Fatal("expected not ready when daemon unhealthy")
	}

	h.isDaemonHealthy = func() bool { return true }
	if !h.IsReadyToUpgradeWithAction(map[string]any{"version": "2.0.0"}) {
		t.Fatal("expected ready when installed != target and daemon healthy")
	}

	if h.IsReadyToUpgradeWithAction(map[string]any{"version": "1.0.0"}) {
		t.Fatal("expected not ready when versions match")
	}

	_ = os.Remove(script)
	if h.IsReadyToUpgradeWithAction(map[string]any{"version": "2.0.0"}) {
		t.Fatal("expected not ready when install.sh missing")
	}
}

func TestIsReadyToUpgrade_BlocksDuringOTA(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "install.sh")
	receipt := filepath.Join(dir, "install-receipt")
	writeFile(t, script, "#!/bin/sh\n")
	writeKV(t, receipt, map[string]string{"installed_version": "1.0.0"})

	h := NewHandler(NewReleaseManager(
		WithPaths(script, receipt, filepath.Join(dir, "previous"), filepath.Join(dir, "cache")),
		WithRunningVersion(func() string { return "1.0.0" }),
	))
	h.isContainer = func() bool { return false }
	h.isDaemonHealthy = func() bool { return true }
	h.markOTAInProgress()

	if h.IsReadyToUpgradeWithAction(map[string]any{"version": "2.0.0"}) {
		t.Fatal("expected not ready during OTA")
	}
}

func TestIsReadyToRollback_RequiresCacheOrReachableURL(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	previous := filepath.Join(dir, "previous-release")
	_ = os.MkdirAll(cacheDir, 0o755)

	writeKV(t, previous, map[string]string{
		"previous_version":      "1.0.0",
		"previous_os":           "linux",
		"previous_arch":         "amd64",
		"previous_download_url": "",
	})

	rm := NewReleaseManager(
		WithPaths(filepath.Join(dir, "install.sh"), filepath.Join(dir, "receipt"), previous, cacheDir),
	)
	h := NewHandler(rm)
	h.isContainer = func() bool { return false }

	if h.IsReadyToRollback() {
		t.Fatal("expected not ready without cache or reachable url")
	}

	cached := filepath.Join(cacheDir, "edgelet-1.0.0-linux-amd64")
	writeFile(t, cached, "binary")
	if !h.IsReadyToRollback() {
		t.Fatal("expected ready when cached binary exists")
	}

	_ = os.Remove(cached)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	writeKV(t, previous, map[string]string{
		"previous_version":      "1.0.0",
		"previous_os":           "linux",
		"previous_arch":         "amd64",
		"previous_download_url": server.URL + "/edgelet-linux-amd64",
	})
	rm = NewReleaseManager(
		WithPaths(filepath.Join(dir, "install.sh"), filepath.Join(dir, "receipt"), previous, cacheDir),
		WithHTTPClient(server.Client()),
	)
	h = NewHandler(rm)
	h.isContainer = func() bool { return false }

	if !h.IsReadyToRollback() {
		t.Fatal("expected ready when previous_download_url is reachable")
	}
}

func TestIsReadyToUpgrade_ContainerComparesImageTag(t *testing.T) {
	h := NewHandler(NewReleaseManager(
		WithRunningVersion(func() string { return "1.2.3" }),
	))
	h.isContainer = func() bool { return true }

	if !h.IsReadyToUpgradeWithAction(map[string]any{"version": "2.0.0"}) {
		t.Fatal("expected container ready when running != target")
	}
	if h.IsReadyToUpgradeWithAction(map[string]any{"version": "1.2.3"}) {
		t.Fatal("expected container not ready when versions match")
	}
}

func TestExecuteChangeVersionScript_LaunchesDetachedInstallSh(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "install.sh")
	logFile := filepath.Join(dir, "invocation.log")
	writeFile(t, script, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \""+logFile+"\"\n")
	_ = os.Chmod(script, 0o755)

	var mu sync.Mutex
	var launched []string
	h := NewHandler(NewReleaseManager(WithPaths(script, filepath.Join(dir, "receipt"), filepath.Join(dir, "prev"), filepath.Join(dir, "cache"))))
	h.isContainer = func() bool { return false }
	h.isDaemonHealthy = func() bool { return true }
	h.startDetached = func(path string, args ...string) error {
		mu.Lock()
		launched = append(launched, path+":"+strings.Join(args, ","))
		mu.Unlock()
		return defaultStartDetached(path, args...)
	}

	writeKV(t, filepath.Join(dir, "receipt"), map[string]string{"installed_version": "1.0.0"})
	err := h.executeChangeVersionScript(
		VersionCommandUpgrade,
		map[string]any{"version": "v2.0.0", "provisionKey": "audit-key"},
		"audit-key",
	)
	if err != nil {
		t.Fatalf("executeChangeVersionScript failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(launched) != 1 {
		t.Fatalf("expected one detached launch, got %#v", launched)
	}
	if !strings.Contains(launched[0], "--upgrade") || !strings.Contains(launched[0], "--version=v2.0.0") {
		t.Fatalf("unexpected launch args: %q", launched[0])
	}
}

func TestChangeVersion_ContainerSkipsInstallScript(t *testing.T) {
	var launched bool
	h := NewHandler(NewReleaseManager())
	h.isContainer = func() bool { return true }
	h.startDetached = func(string, ...string) error {
		launched = true
		return nil
	}

	if err := h.ChangeVersion(map[string]any{"command": "UPGRADE", "version": "2.0.0"}); err != nil {
		t.Fatalf("ChangeVersion failed: %v", err)
	}
	if launched {
		t.Fatal("expected container mode to skip install.sh")
	}
}

func TestNormalizeVersion(t *testing.T) {
	if normalizeVersion("v1.2.3") != "1.2.3" {
		t.Fatal("unexpected normalize result")
	}
}

func TestDefaultDaemonHealthy(t *testing.T) {
	// Smoke test: default hook reads supervisor status without panic.
	_ = defaultDaemonHealthy()
	_ = models.ModuleStatusRunning
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile(%s): %v", path, err)
	}
}

func writeKV(t *testing.T, path string, kv map[string]string) {
	t.Helper()
	var b strings.Builder
	for k, v := range kv {
		_, _ = b.WriteString(k)
		_, _ = b.WriteString("=")
		_, _ = b.WriteString(v)
		_, _ = b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("writeKV(%s): %v", path, err)
	}
}
