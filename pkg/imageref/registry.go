package imageref

import "strings"

// FromCacheRegistry reports whether registryURL is the special from_cache sentinel.
func FromCacheRegistry(registryURL string) bool {
	return strings.EqualFold(strings.TrimSpace(registryURL), "from_cache")
}

// MatchParamsOptional returns registryURL and fromCache for image match/lookup.
// An empty registryURL uses fromCache=true (cache/drift lookup semantics).
func MatchParamsOptional(registryURL string) (url string, fromCache bool) {
	url = strings.TrimSpace(registryURL)
	if url == "" {
		return "", true
	}
	return url, FromCacheRegistry(url)
}

// ResolveForRegistry resolves pull and lookup refs for imageName against registryURL.
// fromCache mirrors pull vs from_cache registry behavior used across process manager.
func ResolveForRegistry(imageName, registryURL string) (pullRef string, lookup []string, fromCache bool) {
	url, fromCache := MatchParamsOptional(registryURL)
	pullRef, lookup = Resolve(imageName, url, fromCache)
	return pullRef, lookup, fromCache
}
