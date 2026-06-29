package engine

import (
	"strings"

	imagetypes "github.com/moby/moby/api/types/image"
)

// SizeUnknown marks a size or count field that could not be determined.
const SizeUnknown = int64(-1)

// ImageSizesFromSummary extracts content and disk usage from a Docker/Moby image summary.
// Content size requires manifest details (ImageList with Manifests=true, API >= 1.47).
func ImageSizesFromSummary(s imagetypes.Summary) (contentSize, diskUsage int64, contentKnown bool) {
	if len(s.Manifests) == 0 {
		if s.Size > 0 {
			return SizeUnknown, s.Size, false
		}
		return SizeUnknown, 0, false
	}

	var content int64
	for _, m := range s.Manifests {
		content += m.Size.Content
	}

	diskUsage = s.Size
	if diskUsage <= 0 {
		for _, m := range s.Manifests {
			diskUsage += m.Size.Total
		}
	}
	return content, diskUsage, true
}

// ImageInUseFromSummary returns the container usage count from a Docker/Moby summary.
func ImageInUseFromSummary(s imagetypes.Summary) int64 {
	if s.Containers < 0 {
		return SizeUnknown
	}
	return s.Containers
}

// IsDigestOnlyImageRef reports whether a containerd/Docker image name is a digest alias.
func IsDigestOnlyImageRef(ref string) bool {
	return strings.HasPrefix(strings.TrimSpace(ref), "sha256:")
}

// PickPrimaryRepoTag chooses the best human-readable repository and tag from references.
func PickPrimaryRepoTag(repoTags []string) (repository, tag string) {
	repository, tag = "<none>", "<none>"
	for _, ref := range repoTags {
		ref = strings.TrimSpace(ref)
		if ref == "" || ref == "<none>:<none>" || IsDigestOnlyImageRef(ref) {
			continue
		}
		return SplitRepoTag(ref)
	}
	return repository, tag
}

// SplitRepoTag splits a reference into repository and tag at the last colon.
func SplitRepoTag(ref string) (repository, tag string) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "<none>:<none>" {
		return "<none>", "<none>"
	}
	idx := strings.LastIndex(ref, ":")
	if idx <= 0 {
		return ref, "<none>"
	}
	return ref[:idx], ref[idx+1:]
}

// LookupImageInUseByID returns the container count for an image ID key.
func LookupImageInUseByID(imageID string, counts map[string]int64) int64 {
	imageID = strings.TrimSpace(imageID)
	if imageID == "" || len(counts) == 0 {
		return 0
	}
	if count, ok := counts[imageID]; ok {
		return count
	}
	short := strings.TrimPrefix(imageID, "sha256:")
	if count, ok := counts["sha256:"+short]; ok {
		return count
	}
	if count, ok := counts[short]; ok {
		return count
	}
	return 0
}

// CountImageInUse counts containers referencing an image by any of its known names or digest.
func CountImageInUse(names []string, digest string, counts map[string]int64) int64 {
	if len(counts) == 0 {
		return 0
	}
	seen := make(map[string]struct{})
	var total int64
	keys := append([]string(nil), names...)
	keys = append(keys, digest)
	if d := strings.TrimPrefix(digest, "sha256:"); d != digest {
		keys = append(keys, "sha256:"+d)
	}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		total += counts[key]
	}
	return total
}
