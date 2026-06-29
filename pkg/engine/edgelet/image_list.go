//go:build linux

package edgelet

import (
	"context"
	"slices"
	"strings"

	"github.com/containerd/containerd/v2/client"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

type imageDigestGroup struct {
	img   client.Image
	names []string
}

func (e *Engine) listImageInUseCounts(ctx context.Context) (map[string]int64, error) {
	containers, err := e.client.Containers(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(containers))
	for _, c := range containers {
		info, infoErr := c.Info(ctx)
		if infoErr != nil {
			continue
		}
		if ref := strings.TrimSpace(info.Image); ref != "" {
			counts[ref]++
		}
	}
	return counts, nil
}

func groupImagesByDigest(imgs []client.Image) map[string]*imageDigestGroup {
	byDigest := make(map[string]*imageDigestGroup, len(imgs))
	for _, img := range imgs {
		digest := img.Target().Digest.String()
		group, ok := byDigest[digest]
		if !ok {
			group = &imageDigestGroup{img: img}
			byDigest[digest] = group
		}
		if name := strings.TrimSpace(img.Name()); name != "" {
			group.names = append(group.names, name)
		}
	}
	return byDigest
}

func imageInfoFromDigestGroup(ctx context.Context, c *client.Client, digest string, group *imageDigestGroup, inUseCounts map[string]int64, inUseKnown bool) engine.ImageInfo {
	contentSize, diskUsage := imageDiskUsage(ctx, c, group.img)

	repoTags := append([]string(nil), group.names...)
	repository, tag := engine.PickPrimaryRepoTag(repoTags)
	shortID := strings.TrimPrefix(digest, "sha256:")
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	inUse := engine.SizeUnknown
	if inUseKnown {
		inUse = engine.CountImageInUse(group.names, digest, inUseCounts)
	}

	return engine.ImageInfo{
		ID:               digest,
		RepoTags:         repoTags,
		ShortID:          shortID,
		Repository:       repository,
		Tag:              tag,
		Digest:           digest,
		CreatedAt:        group.img.Metadata().CreatedAt.UTC(),
		ContentSizeBytes: contentSize,
		DiskUsageBytes:   diskUsage,
		InUse:            inUse,
		Engine:           "edgelet",
	}
}

func sortImageInfos(items []engine.ImageInfo) {
	slices.SortFunc(items, func(a, b engine.ImageInfo) int {
		if !a.CreatedAt.Equal(b.CreatedAt) {
			if a.CreatedAt.After(b.CreatedAt) {
				return -1
			}
			return 1
		}
		if a.Repository != b.Repository {
			return strings.Compare(a.Repository, b.Repository)
		}
		return strings.Compare(a.Tag, b.Tag)
	})
}
