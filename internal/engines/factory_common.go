package engines

import (
	"os"

	"github.com/datasance/edgelet/internal/utils/logging"
)

var factoryLogger = logging.NewModuleLogger("ContainerEngine")

// warnIfExternalRuntimePresent logs a warning when Docker or Podman sockets are
// found on the host while the edgelet embedded engine is selected.
func warnIfExternalRuntimePresent() {
	sockets := []struct {
		name string
		path string
	}{
		{"Docker", "/var/run/docker.sock"},
		{"Podman", "/run/podman/podman.sock"},
		{"Podman (user)", "/run/user/0/podman/podman.sock"},
	}

	for _, s := range sockets {
		if _, err := os.Stat(s.path); err == nil {
			factoryLogger.Warnf(
				"%s socket detected at %s while containerEngine=edgelet is selected. "+
					"The edgelet engine uses isolated private paths (/var/lib/edgelet-containerd/, "+
					"/run/edgelet/containerd.sock) and will not interfere with %s.",
				s.name, s.path, s.name,
			)
		}
	}
}
