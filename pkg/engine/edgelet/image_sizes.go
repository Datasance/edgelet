//go:build linux

package edgelet

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/image-spec/identity"
)

const defaultSnapshotter = "overlayfs"

func imageContentSize(ctx context.Context, img client.Image) (int64, error) {
	return img.Usage(ctx)
}

func imageUnpackedSize(ctx context.Context, c *client.Client, img client.Image, snapshotter string) (int64, error) {
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return 0, fmt.Errorf("rootfs: %w", err)
	}
	if len(diffIDs) == 0 {
		return 0, nil
	}

	sn := c.SnapshotService(snapshotter)

	chainID := identity.ChainID(diffIDs).String()
	usage, err := snapshotTotalUsage(ctx, sn, chainID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return usage.Size, nil
}

func imageDiskUsage(ctx context.Context, c *client.Client, img client.Image) (contentSize int64, diskUsage int64) {
	content, err := imageContentSize(ctx, img)
	if err != nil {
		return 0, 0
	}
	contentSize = content

	unpacked, err := imageUnpackedSize(ctx, c, img, defaultSnapshotter)
	if err != nil {
		return contentSize, contentSize
	}
	return contentSize, contentSize + unpacked
}

func snapshotTotalUsage(ctx context.Context, snapshotter snapshots.Snapshotter, snapshotID string) (snapshots.Usage, error) {
	var total snapshots.Usage
	next := snapshotID
	for next != "" {
		usage, err := snapshotter.Usage(ctx, next)
		if err != nil {
			if errdefs.IsNotFound(err) {
				return total, err
			}
			return total, err
		}
		total.Size += usage.Size
		total.Inodes += usage.Inodes

		info, err := snapshotter.Stat(ctx, next)
		if err != nil {
			return total, err
		}
		next = info.Parent
	}
	return total, nil
}
