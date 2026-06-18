package imageref

import "testing"

func TestFromCacheRegistry(t *testing.T) {
	if !FromCacheRegistry("from_cache") {
		t.Fatal("expected from_cache sentinel")
	}
	if FromCacheRegistry("quay.io") {
		t.Fatal("quay.io is not from_cache")
	}
}

func TestMatchParamsOptional_EmptyUsesFromCache(t *testing.T) {
	url, fromCache := MatchParamsOptional("")
	if url != "" || !fromCache {
		t.Fatalf("empty registry: url=%q fromCache=%v", url, fromCache)
	}
}

func TestMatchParamsOptional_QuayRegistry(t *testing.T) {
	url, fromCache := MatchParamsOptional("quay.io")
	if url != "quay.io" || fromCache {
		t.Fatalf("quay: url=%q fromCache=%v", url, fromCache)
	}
}

func TestResolveForRegistry_QuayInjectsHost(t *testing.T) {
	pullRef, lookup, fromCache := ResolveForRegistry("org/app:v1", "quay.io")
	if fromCache {
		t.Fatal("quay registry should not use from_cache pull semantics")
	}
	if pullRef != "quay.io/org/app:v1" {
		t.Fatalf("pullRef=%q", pullRef)
	}
	found := false
	for _, ref := range lookup {
		if ref == "quay.io/org/app:v1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected quay qualified alias in lookup, got %v", lookup)
	}
}

func TestResolveForRegistry_FromCacheKeepsShortPullRef(t *testing.T) {
	pullRef, _, fromCache := ResolveForRegistry("org/app:v1", "from_cache")
	if !fromCache {
		t.Fatal("expected from_cache semantics")
	}
	if pullRef != "org/app:v1" {
		t.Fatalf("from_cache pullRef=%q want org/app:v1", pullRef)
	}
}
