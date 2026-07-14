package runtimeapi

import (
	"errors"
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

func testAlpineImage() engine.ImageInfo {
	return engine.ImageInfo{
		ID:         "sha256:6baf43584bcb78f2e5847d1de515f23499913ac9f12bdf834811a3145eb11ca1",
		Digest:     "sha256:6baf43584bcb78f2e5847d1de515f23499913ac9f12bdf834811a3145eb11ca1",
		ShortID:    "6baf43584bcb",
		Repository: "alpine",
		Tag:        "3.19",
		RepoTags:   []string{"docker.io/library/alpine:3.19"},
		Engine:     "edgelet",
	}
}

func TestResolveImageDeleteRefFromList_ShortNameTag(t *testing.T) {
	ref, err := resolveImageDeleteRefFromList("alpine:3.19", []engine.ImageInfo{testAlpineImage()})
	if err != nil {
		t.Fatalf("resolveImageDeleteRefFromList: %v", err)
	}
	if ref != "docker.io/library/alpine:3.19" {
		t.Fatalf("expected qualified repo tag delete ref, got %q", ref)
	}
}

func TestResolveImageDeleteRefFromList_FullRepoTag(t *testing.T) {
	ref, err := resolveImageDeleteRefFromList("docker.io/library/alpine:3.19", []engine.ImageInfo{testAlpineImage()})
	if err != nil {
		t.Fatalf("resolveImageDeleteRefFromList: %v", err)
	}
	if ref != "docker.io/library/alpine:3.19" {
		t.Fatalf("expected repo tag delete ref, got %q", ref)
	}
}

func TestResolveImageDeleteRefFromList_ShortID(t *testing.T) {
	ref, err := resolveImageDeleteRefFromList("6baf43584bcb", []engine.ImageInfo{testAlpineImage()})
	if err != nil {
		t.Fatalf("resolveImageDeleteRefFromList: %v", err)
	}
	if ref != "docker.io/library/alpine:3.19" {
		t.Fatalf("expected delete ref via short id, got %q", ref)
	}
}

func TestResolveImageDeleteRefFromList_IDPrefix(t *testing.T) {
	ref, err := resolveImageDeleteRefFromList("6baf4", []engine.ImageInfo{testAlpineImage()})
	if err != nil {
		t.Fatalf("resolveImageDeleteRefFromList: %v", err)
	}
	if ref != "docker.io/library/alpine:3.19" {
		t.Fatalf("expected delete ref via id prefix, got %q", ref)
	}
}

func TestResolveImageDeleteRefFromList_FullDigest(t *testing.T) {
	ref, err := resolveImageDeleteRefFromList("sha256:6baf43584bcb78f2e5847d1de515f23499913ac9f12bdf834811a3145eb11ca1", []engine.ImageInfo{testAlpineImage()})
	if err != nil {
		t.Fatalf("resolveImageDeleteRefFromList: %v", err)
	}
	if ref != "docker.io/library/alpine:3.19" {
		t.Fatalf("expected delete ref via digest, got %q", ref)
	}
}

func TestResolveImageDeleteRefFromList_AmbiguousPrefix(t *testing.T) {
	images := []engine.ImageInfo{
		{
			ID:       "sha256:aaa1111111111111111111111111111111111111111111111111111111111",
			Digest:   "sha256:aaa1111111111111111111111111111111111111111111111111111111111",
			ShortID:  "aaa111111111",
			RepoTags: []string{"example.com/one:1"},
		},
		{
			ID:       "sha256:aaa2222222222222222222222222222222222222222222222222222222222",
			Digest:   "sha256:aaa2222222222222222222222222222222222222222222222222222222222",
			ShortID:  "aaa222222222",
			RepoTags: []string{"example.com/two:1"},
		},
	}
	_, err := resolveImageDeleteRefFromList("aaa", images)
	if err == nil {
		t.Fatal("expected ambiguous selector error")
	}
	var amb *ErrAmbiguousImageSelector
	if !errors.As(err, &amb) {
		t.Fatalf("expected ErrAmbiguousImageSelector, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "ambiguous image selector") {
		t.Fatalf("expected ambiguous error message, got %v", err)
	}
}

func TestResolveImageDeleteRefFromList_NotFound(t *testing.T) {
	_, err := resolveImageDeleteRefFromList("missing:1.0", []engine.ImageInfo{testAlpineImage()})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestResolveImageDeleteRefFromList_RequiresSelector(t *testing.T) {
	_, err := resolveImageDeleteRefFromList("   ", []engine.ImageInfo{testAlpineImage()})
	if err == nil || !strings.Contains(err.Error(), "selector is required") {
		t.Fatalf("expected selector validation error, got %v", err)
	}
}

func TestFacadeRemoveImage_ValidatesSelector(t *testing.T) {
	f := NewFacade()
	if _, err := f.RemoveImage("   "); err == nil || !strings.Contains(err.Error(), "selector is required") {
		t.Fatalf("expected selector validation error, got: %v", err)
	}
}

func TestFacadeResolveImageDeleteRef_UsesListImages(t *testing.T) {
	orig := processmanagerListImages
	t.Cleanup(func() { processmanagerListImages = orig })
	processmanagerListImages = func() ([]engine.ImageInfo, error) {
		return []engine.ImageInfo{testAlpineImage()}, nil
	}

	f := NewFacade()
	ref, err := f.ResolveImageDeleteRef("alpine:3.19")
	if err != nil {
		t.Fatalf("ResolveImageDeleteRef: %v", err)
	}
	if ref != "docker.io/library/alpine:3.19" {
		t.Fatalf("expected qualified repo tag, got %q", ref)
	}
}
