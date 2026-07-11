//go:build linux

package edgelet

import (
	"fmt"
	"io"

	"github.com/containerd/containerd/v2/pkg/archive/compression"
)

// decompressImageArchive returns a reader for containerd Import. It accepts
// uncompressed tar archives and gzip-compressed tar (.tar.gz), matching
// docker/podman load behavior.
func decompressImageArchive(archive io.Reader) (io.ReadCloser, error) {
	decompressed, err := compression.DecompressStream(archive)
	if err != nil {
		return nil, fmt.Errorf("decompress image archive: %w", err)
	}
	return decompressed, nil
}
