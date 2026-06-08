//go:build linux && cgo

package containerd

// The blank imports here register all containerd plugins as in-process components.
// Without these imports, containerd would start but have no working runtime,
// snapshotter, metadata store, CRI, or service handlers.
//
// Mirrors github.com/containerd/containerd/v2/cmd/containerd/builtins (builtins.go,
// cri.go) plus the overlay snapshotter and walking diff plugins used by edgelet.
import (
	_ "github.com/containerd/containerd/v2/core/runtime/v2"                  // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/content/local/plugin"     // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/cri"                      // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/cri/images"               // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/cri/runtime"              // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/diff/walking/plugin"      // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/events"                   // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/gc"                       // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/imageverifier"            // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/leases"                   // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/metadata"                 // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/mount"                    // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/nri"                      // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/restart"                  // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/sandbox"                  // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/containers"      // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/content"         // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/diff"            // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/events"          // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/healthcheck"     // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/images"          // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/introspection"   // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/leases"          // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/mounts"          // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/namespaces"      // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/opt"             // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/sandbox"         // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/snapshots"       // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/streaming"       // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/tasks"           // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/transfer"        // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/version"         // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/services/warning"         // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/snapshots/overlay/plugin" // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/streaming"                // register containerd in-process plugin
	_ "github.com/containerd/containerd/v2/plugins/transfer"                 // register containerd in-process plugin
)
