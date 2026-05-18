//go:build linux

package iofog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildHostsFile_WritesOnlyBaselineAndUserExtraHosts(t *testing.T) {
	target := filepath.Join(t.TempDir(), "hosts", "sample")
	extraHosts := []string{
		"example.local:10.10.10.10",
		"another.local:10.10.10.11",
	}
	if err := buildHostsFile(target, extraHosts); err != nil {
		t.Fatalf("buildHostsFile returned error: %v", err)
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed reading hosts file: %v", err)
	}
	content := string(raw)

	baseline := []string{
		"127.0.0.1\tlocalhost",
		"::1\t\tlocalhost ip6-localhost ip6-loopback",
		"fe00::0\tip6-localnet",
		"ff00::0\tip6-mcastprefix",
		"ff02::1\tip6-allnodes",
		"ff02::2\tip6-allrouters",
	}
	for _, line := range baseline {
		if !strings.Contains(content, line) {
			t.Fatalf("expected baseline line %q in hosts file, got:\n%s", line, content)
		}
	}
	if !strings.Contains(content, "10.10.10.10\texample.local") {
		t.Fatalf("expected user extra host for example.local, got:\n%s", content)
	}
	if !strings.Contains(content, "10.10.10.11\tanother.local") {
		t.Fatalf("expected user extra host for another.local, got:\n%s", content)
	}
}

func TestBuildHostsFile_DoesNotWriteLegacyOrReservedAliases(t *testing.T) {
	target := filepath.Join(t.TempDir(), "hosts", "sample")
	if err := buildHostsFile(target, nil); err != nil {
		t.Fatalf("buildHostsFile returned error: %v", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed reading hosts file: %v", err)
	}
	content := string(raw)

	forbidden := []string{
		"service.local",
		"iofog.default.svc.bridge.local",
		"router.default.svc.bridge.local",
		"nats.default.svc.bridge.local",
		"\tiofog\n",
	}
	for _, token := range forbidden {
		if strings.Contains(content, token) {
			t.Fatalf("hosts file should not contain %q, got:\n%s", token, content)
		}
	}
}

func TestBuildHostsFile_SkipsInvalidExtraHostsEntries(t *testing.T) {
	target := filepath.Join(t.TempDir(), "hosts", "sample")
	extraHosts := []string{
		"",
		"  ",
		"missing-colon",
		":10.0.0.1",
		"empty-ip:",
		"ok.local:10.0.0.9",
		"also-ok.local : 10.0.0.10",
	}
	if err := buildHostsFile(target, extraHosts); err != nil {
		t.Fatalf("buildHostsFile returned error: %v", err)
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed reading hosts file: %v", err)
	}
	content := string(raw)

	if !strings.Contains(content, "10.0.0.9\tok.local") {
		t.Fatalf("expected valid host entry for ok.local, got:\n%s", content)
	}
	if !strings.Contains(content, "10.0.0.10\talso-ok.local") {
		t.Fatalf("expected valid host entry for also-ok.local, got:\n%s", content)
	}
	if strings.Contains(content, "missing-colon") || strings.Contains(content, "empty-ip") {
		t.Fatalf("invalid host entries should be skipped, got:\n%s", content)
	}
}
