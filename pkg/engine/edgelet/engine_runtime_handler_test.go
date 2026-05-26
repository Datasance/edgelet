//go:build linux

package iofog

import (
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/dnsresolver"
	"github.com/datasance/edgelet/internal/models"
)

func TestCreateContainer_RejectsUnknownRuntimeBeforeCRI(t *testing.T) {
	e := New("")
	ms := models.NewMicroservice("ms-1", "busybox:latest")
	runtime := "does-not-exist"
	ms.Runtime = &runtime

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CreateContainer should not panic for unknown runtime: %v", r)
		}
	}()

	_, err := e.CreateContainer(ms, "host")
	if err == nil {
		t.Fatal("expected unknown runtime error")
	}
	if !strings.Contains(err.Error(), "unknown runtime class") {
		t.Fatalf("expected unknown runtime class error, got: %v", err)
	}
}

func TestDNSScopeFromMicroservice_UsesResolvedScope(t *testing.T) {
	ms := models.NewMicroservice("ms-2", "busybox:latest")
	ms.ApplicationName = "edgelet"
	ms.HostNetworkMode = false
	if got := dnsScopeFromMicroservice(ms); got != dnsresolver.ScopeLocal {
		t.Fatalf("expected local dns scope, got %q", got)
	}

	ms.HostNetworkMode = true
	if got := dnsScopeFromMicroservice(ms); got != dnsresolver.ScopeManaged {
		t.Fatalf("expected managed dns scope for host-network bypass, got %q", got)
	}
}

func TestDNSScopeFromWorkloadScope(t *testing.T) {
	if got := dnsScopeFromWorkloadScope("local"); got != dnsresolver.ScopeLocal {
		t.Fatalf("expected local dns scope, got %q", got)
	}
	if got := dnsScopeFromWorkloadScope("managed"); got != dnsresolver.ScopeManaged {
		t.Fatalf("expected managed dns scope, got %q", got)
	}
}
