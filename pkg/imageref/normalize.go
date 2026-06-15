package imageref

import (
	"strings"
)

// SanitizeRegistryHost normalizes a registry URL-like value to a plain host[:port].
// Examples:
//   - https://docker.io/ -> docker.io
//   - http://registry.local:5000/v2 -> registry.local:5000
func SanitizeRegistryHost(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")
	// Registry URL may include path parts; image refs need only host[:port].
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// HasRegistryHost returns true when imageRef already contains an explicit registry host.
// Docker reference heuristic: first component before '/' is considered a host
// when it contains '.' or ':' or equals localhost.
func HasRegistryHost(imageRef string) bool {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return false
	}
	slash := strings.Index(ref, "/")
	if slash < 0 {
		return false
	}
	first := ref[:slash]
	return strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost"
}

// Resolve returns pull and lookup references for an image.
// - fromCache=true never mutates the returned pull ref (it is not used for pulling)
// - lookup refs include safe aliases to improve local cache matching.
func Resolve(imageRef, registryURL string, fromCache bool) (string, []string) {
	orig := strings.TrimSpace(imageRef)
	if orig == "" {
		return "", []string{}
	}
	host := SanitizeRegistryHost(registryURL)
	hasHost := HasRegistryHost(orig)

	pullRef := orig
	if !fromCache && !hasHost && host != "" {
		pullRef = host + "/" + orig
	}

	lookup := make([]string, 0, 8)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, e := range lookup {
			if e == v {
				return
			}
		}
		lookup = append(lookup, v)
	}
	addWithLatest := func(v string) {
		add(v)
		if !strings.Contains(v, "@sha256:") && !hasTag(v) {
			add(v + ":latest")
		}
	}

	addWithLatest(orig)
	if pullRef != orig {
		addWithLatest(pullRef)
	}
	// For from_cache, include host-injected alias as a lookup candidate.
	if fromCache && !hasHost && host != "" {
		addWithLatest(host + "/" + orig)
	}

	// Safe Docker Hub aliases for hostless names.
	if !hasHost {
		addWithLatest("docker.io/" + orig)
		if singleComponentRepo(orig) {
			addWithLatest("docker.io/library/" + orig)
		}
	}

	// Docker tags official library images under short names (alpine:3.19) even when
	// the manifest uses the fully qualified ref (docker.io/library/alpine:3.19).
	if strings.HasPrefix(orig, "docker.io/library/") {
		addWithLatest(strings.TrimPrefix(orig, "docker.io/library/"))
	} else if strings.HasPrefix(orig, "docker.io/") {
		addWithLatest(strings.TrimPrefix(orig, "docker.io/"))
	}

	return pullRef, lookup
}

// Match reports whether two image refs refer to the same image when resolved
// against registryURL. fromCache mirrors launch/pull behavior for from_cache registries.
func Match(a, b, registryURL string, fromCache bool) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if strings.EqualFold(a, b) {
		return true
	}
	pullA, aliasesA := Resolve(a, registryURL, fromCache)
	pullB, aliasesB := Resolve(b, registryURL, fromCache)
	if pullA != "" && pullA == pullB {
		return true
	}
	host := SanitizeRegistryHost(registryURL)
	if !fromCache && host != "" && host != "docker.io" {
		aliasesA = qualifiedRegistryAliases(aliasesA, host)
		aliasesB = qualifiedRegistryAliases(aliasesB, host)
	}
	for _, la := range aliasesA {
		for _, lb := range aliasesB {
			if la == lb {
				return true
			}
		}
	}
	return false
}

func qualifiedRegistryAliases(aliases []string, host string) []string {
	prefix := host + "/"
	out := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if HasRegistryHost(alias) && strings.HasPrefix(alias, prefix) {
			out = append(out, alias)
		}
	}
	return out
}

func hasTag(ref string) bool {
	// tag is defined on the last path component.
	last := ref
	if idx := strings.LastIndex(last, "/"); idx >= 0 {
		last = last[idx+1:]
	}
	return strings.Contains(last, ":")
}

func singleComponentRepo(ref string) bool {
	if idx := strings.Index(ref, "@sha256:"); idx >= 0 {
		ref = ref[:idx]
	}
	// remove tag from last component for slash counting
	if hasTag(ref) {
		if idx := strings.LastIndex(ref, ":"); idx >= 0 {
			ref = ref[:idx]
		}
	}
	return !strings.Contains(ref, "/")
}
