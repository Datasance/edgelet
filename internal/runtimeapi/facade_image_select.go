package runtimeapi

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
	"github.com/eclipse-iofog/edgelet/pkg/imageref"
)

var imageIDPrefixPattern = regexp.MustCompile(`^[a-f0-9]{3,64}$`)

// ErrAmbiguousImageSelector indicates an image selector matched multiple images.
type ErrAmbiguousImageSelector struct {
	Selector string
	Matches  []string
}

func (e *ErrAmbiguousImageSelector) Error() string {
	if e == nil || len(e.Matches) == 0 {
		return "ambiguous image selector"
	}
	return fmt.Sprintf("ambiguous image selector %q; candidates: %s", e.Selector, strings.Join(e.Matches, ", "))
}

type imageDeleteCandidate struct {
	deleteRef string
	label     string
}

// ResolveImageDeleteRef resolves a user selector to an engine delete reference.
func (f *Facade) ResolveImageDeleteRef(selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", errors.New("selector is required")
	}
	items, err := f.listEngineImages()
	if err != nil {
		return "", err
	}
	return resolveImageDeleteRefFromList(selector, items)
}

func (f *Facade) listEngineImages() ([]engine.ImageInfo, error) {
	return processmanagerListImages()
}

// processmanagerListImages is overridden in tests.
var processmanagerListImages = func() ([]engine.ImageInfo, error) {
	return processmanager.GetInstance().ListImages()
}

func resolveImageDeleteRefFromList(selector string, items []engine.ImageInfo) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", errors.New("selector is required")
	}

	candidates := make([]imageDeleteCandidate, 0, 1)
	seen := make(map[string]struct{})
	for _, item := range items {
		deleteRef, ok := matchImageSelector(item, selector)
		if !ok {
			continue
		}
		if _, exists := seen[deleteRef]; exists {
			continue
		}
		seen[deleteRef] = struct{}{}
		candidates = append(candidates, imageDeleteCandidate{
			deleteRef: deleteRef,
			label:     imageDisplayLabel(item),
		})
	}

	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("image not found: %s", selector)
	case 1:
		return candidates[0].deleteRef, nil
	default:
		labels := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			labels = append(labels, candidate.label)
		}
		slices.Sort(labels)
		if len(labels) > 5 {
			labels = labels[:5]
		}
		return "", &ErrAmbiguousImageSelector{Selector: selector, Matches: labels}
	}
}

func matchImageSelector(item engine.ImageInfo, selector string) (deleteRef string, matched bool) {
	if imageRefEquals(item.ID, selector) || imageRefEquals(item.Digest, selector) {
		return preferredImageDeleteRef(item), true
	}
	if strings.EqualFold(strings.TrimSpace(item.ShortID), selector) {
		return preferredImageDeleteRef(item), true
	}

	for _, ref := range item.RepoTags {
		if strings.EqualFold(strings.TrimSpace(ref), selector) {
			return preferredDeleteRefForTag(item, ref), true
		}
	}

	if display := imageDisplayRef(item); display != "" && strings.EqualFold(display, selector) {
		return preferredImageDeleteRef(item), true
	}

	if imageSelectorLooksLikeNameRef(selector) {
		for _, ref := range item.RepoTags {
			if imageref.Match(selector, ref, "docker.io", true) {
				return preferredDeleteRefForTag(item, ref), true
			}
		}
		if display := imageDisplayRef(item); display != "" && imageref.Match(selector, display, "docker.io", true) {
			return preferredImageDeleteRef(item), true
		}
	}

	if looksLikeImageIDPrefix(selector) {
		if imageIDMatchesPrefix(item.ID, selector) ||
			imageIDMatchesPrefix(item.ShortID, selector) ||
			imageIDMatchesPrefix(item.Digest, selector) {
			return preferredImageDeleteRef(item), true
		}
	}

	return "", false
}

func preferredImageDeleteRef(item engine.ImageInfo) string {
	for _, ref := range item.RepoTags {
		ref = strings.TrimSpace(ref)
		if ref != "" && !engine.IsDigestOnlyImageRef(ref) {
			return ref
		}
	}
	if id := strings.TrimSpace(item.ID); id != "" {
		return id
	}
	return strings.TrimSpace(item.Digest)
}

func preferredDeleteRefForTag(item engine.ImageInfo, matchedTag string) string {
	matchedTag = strings.TrimSpace(matchedTag)
	if matchedTag != "" && !engine.IsDigestOnlyImageRef(matchedTag) {
		return matchedTag
	}
	return preferredImageDeleteRef(item)
}

func imageDisplayRef(item engine.ImageInfo) string {
	repo := strings.TrimSpace(item.Repository)
	tag := strings.TrimSpace(item.Tag)
	if repo == "" || repo == "<none>" || tag == "" || tag == "<none>" {
		return ""
	}
	return repo + ":" + tag
}

func imageDisplayLabel(item engine.ImageInfo) string {
	if display := imageDisplayRef(item); display != "" {
		return display
	}
	if shortID := strings.TrimSpace(item.ShortID); shortID != "" {
		return shortID
	}
	id := strings.TrimSpace(item.ID)
	if len(id) > 19 {
		return id[:19] + "…"
	}
	return id
}

func imageSelectorLooksLikeNameRef(selector string) bool {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return false
	}
	return strings.Contains(selector, ":") || strings.Contains(selector, "/")
}

func imageRefEquals(a, b string) bool {
	a = strings.TrimSpace(strings.ToLower(a))
	b = strings.TrimSpace(strings.ToLower(b))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.TrimPrefix(a, "sha256:") == strings.TrimPrefix(b, "sha256:")
}

func looksLikeImageIDPrefix(selector string) bool {
	normalized := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(selector), "sha256:"))
	return imageIDPrefixPattern.MatchString(normalized)
}

func imageIDMatchesPrefix(imageID, prefix string) bool {
	imageID = strings.TrimSpace(strings.ToLower(imageID))
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if imageID == "" || prefix == "" {
		return false
	}
	if strings.HasPrefix(imageID, prefix) {
		return true
	}
	return strings.HasPrefix(strings.TrimPrefix(imageID, "sha256:"), strings.TrimPrefix(prefix, "sha256:"))
}
