package engine

import "testing"

func TestSupportsInPlaceRestart_Docker(t *testing.T) {
	if !SupportsInPlaceRestart("docker") {
		t.Fatal("expected docker to support in-place restart")
	}
}

func TestSupportsInPlaceRestart_Podman(t *testing.T) {
	if !SupportsInPlaceRestart("podman") {
		t.Fatal("expected podman to support in-place restart")
	}
}

func TestSupportsInPlaceRestart_Iofog(t *testing.T) {
	if SupportsInPlaceRestart("iofog") {
		t.Fatal("expected iofog to require recreate on restart")
	}
}

func TestSupportsInPlaceRestart_Unknown(t *testing.T) {
	if SupportsInPlaceRestart("unknown-engine") {
		t.Fatal("expected unknown engine to default to recreate-required")
	}
}
