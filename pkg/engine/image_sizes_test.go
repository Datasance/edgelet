package engine

import (
	"testing"

	imagetypes "github.com/moby/moby/api/types/image"
)

func TestImageSizesFromSummary(t *testing.T) {
	t.Run("manifests present", func(t *testing.T) {
		m1 := imagetypes.ManifestSummary{}
		m1.Size.Content = 100
		m1.Size.Total = 300
		m2 := imagetypes.ManifestSummary{}
		m2.Size.Content = 50
		m2.Size.Total = 200
		content, disk, known := ImageSizesFromSummary(imagetypes.Summary{
			Size:      500,
			Manifests: []imagetypes.ManifestSummary{m1, m2},
		})
		if !known || content != 150 || disk != 500 {
			t.Fatalf("got content=%d disk=%d known=%v", content, disk, known)
		}
	})
	t.Run("no manifests", func(t *testing.T) {
		content, disk, known := ImageSizesFromSummary(imagetypes.Summary{Size: 999})
		if known || content != SizeUnknown || disk != 999 {
			t.Fatalf("got content=%d disk=%d known=%v", content, disk, known)
		}
	})
}

func TestPickPrimaryRepoTag(t *testing.T) {
	repo, tag := PickPrimaryRepoTag([]string{
		"sha256:abc123",
		"docker.io/library/nginx:latest",
	})
	if repo != "docker.io/library/nginx" || tag != "latest" {
		t.Fatalf("got repo=%q tag=%q", repo, tag)
	}
}

func TestLookupImageInUseByID(t *testing.T) {
	counts := map[string]int64{
		"sha256:deadbeef": 2,
	}
	if got := LookupImageInUseByID("sha256:deadbeef", counts); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}

func TestCountImageInUse(t *testing.T) {
	counts := map[string]int64{
		"docker.io/app:v1": 2,
		"sha256:deadbeef":  1,
	}
	if got := CountImageInUse([]string{"docker.io/app:v1"}, "sha256:deadbeef", counts); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}
