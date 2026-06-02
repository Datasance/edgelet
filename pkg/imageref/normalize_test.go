package imageref

import "testing"

func TestSanitizeRegistryHost(t *testing.T) {
	tests := map[string]string{
		"https://docker.io/":            "docker.io",
		"http://registry.local:5000/v2": "registry.local:5000",
		"registry.example.com/ns/path":  "registry.example.com",
		"  ghcr.io  ":                   "ghcr.io",
		"from_cache":                    "from_cache",
		"":                              "",
	}
	for in, want := range tests {
		if got := SanitizeRegistryHost(in); got != want {
			t.Fatalf("SanitizeRegistryHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolvePublicShortRef(t *testing.T) {
	pullRef, lookup := Resolve("natsio/nats-box:latest", "https://docker.io/", false)
	if pullRef != "docker.io/natsio/nats-box:latest" {
		t.Fatalf("unexpected pullRef: %s", pullRef)
	}
	if len(lookup) < 2 {
		t.Fatalf("expected lookup aliases, got %v", lookup)
	}
}

func TestResolveFromCacheDoesNotMutatePullRef(t *testing.T) {
	pullRef, lookup := Resolve("natsio/nats-box:latest", "from_cache", true)
	if pullRef != "natsio/nats-box:latest" {
		t.Fatalf("from_cache should keep pullRef unchanged, got %s", pullRef)
	}
	foundDockerAlias := false
	for _, v := range lookup {
		if v == "docker.io/natsio/nats-box:latest" {
			foundDockerAlias = true
			break
		}
	}
	if !foundDockerAlias {
		t.Fatalf("expected docker.io alias in lookup candidates, got %v", lookup)
	}
}

func TestResolveKeepsQualifiedRef(t *testing.T) {
	pullRef, _ := Resolve("ghcr.io/datasance/nats:2.12.4", "docker.io", false)
	if pullRef != "ghcr.io/datasance/nats:2.12.4" {
		t.Fatalf("qualified ref should remain unchanged, got %s", pullRef)
	}
}

func TestResolveDockerHubFullyQualifiedIncludesShortAlias(t *testing.T) {
	_, lookup := Resolve("docker.io/library/alpine:3.19", "from_cache", true)
	foundShort := false
	for _, v := range lookup {
		if v == "alpine:3.19" {
			foundShort = true
			break
		}
	}
	if !foundShort {
		t.Fatalf("expected short docker hub alias alpine:3.19 in lookup, got %v", lookup)
	}
}
