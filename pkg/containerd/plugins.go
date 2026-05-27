//go:build linux && cgo

package edgeletcontainerdd

// The blank imports here register all containerd plugins as in-process components.
// Without these imports, containerd would start but have no working runtime,
// snapshotter, metadata store, CRI, or service handlers.
//
// Mirrors github.com/containerd/containerd/v2/cmd/containerd/builtins (builtins.go,
// cri.go) plus the overlay snapshotter and walking diff plugins used by edgelet.
import (
	_ "github.com/containerd/containerd/v2/core/runtime/v2"
	_ "github.com/containerd/containerd/v2/plugins/content/local/plugin"
	_ "github.com/containerd/containerd/v2/plugins/cri"
	_ "github.com/containerd/containerd/v2/plugins/cri/images"
	_ "github.com/containerd/containerd/v2/plugins/cri/runtime"
	_ "github.com/containerd/containerd/v2/plugins/diff/walking/plugin"
	_ "github.com/containerd/containerd/v2/plugins/events"
	_ "github.com/containerd/containerd/v2/plugins/gc"
	_ "github.com/containerd/containerd/v2/plugins/imageverifier"
	_ "github.com/containerd/containerd/v2/plugins/leases"
	_ "github.com/containerd/containerd/v2/plugins/metadata"
	_ "github.com/containerd/containerd/v2/plugins/mount"
	_ "github.com/containerd/containerd/v2/plugins/nri"
	_ "github.com/containerd/containerd/v2/plugins/restart"
	_ "github.com/containerd/containerd/v2/plugins/sandbox"
	_ "github.com/containerd/containerd/v2/plugins/services/containers"
	_ "github.com/containerd/containerd/v2/plugins/services/content"
	_ "github.com/containerd/containerd/v2/plugins/services/diff"
	_ "github.com/containerd/containerd/v2/plugins/services/events"
	_ "github.com/containerd/containerd/v2/plugins/services/healthcheck"
	_ "github.com/containerd/containerd/v2/plugins/services/images"
	_ "github.com/containerd/containerd/v2/plugins/services/introspection"
	_ "github.com/containerd/containerd/v2/plugins/services/leases"
	_ "github.com/containerd/containerd/v2/plugins/services/mounts"
	_ "github.com/containerd/containerd/v2/plugins/services/namespaces"
	_ "github.com/containerd/containerd/v2/plugins/services/opt"
	_ "github.com/containerd/containerd/v2/plugins/services/sandbox"
	_ "github.com/containerd/containerd/v2/plugins/services/snapshots"
	_ "github.com/containerd/containerd/v2/plugins/services/streaming"
	_ "github.com/containerd/containerd/v2/plugins/services/tasks"
	_ "github.com/containerd/containerd/v2/plugins/services/transfer"
	_ "github.com/containerd/containerd/v2/plugins/services/version"
	_ "github.com/containerd/containerd/v2/plugins/services/warning"
	_ "github.com/containerd/containerd/v2/plugins/snapshots/overlay/plugin"
	_ "github.com/containerd/containerd/v2/plugins/streaming"
	_ "github.com/containerd/containerd/v2/plugins/transfer"
)
