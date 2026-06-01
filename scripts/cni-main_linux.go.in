//go:build linux

// Edgelet-minimal CNI multicall (bridge, host-local, loopback, portmap only).
// Copied over rancher/plugins main_linux.go during scripts/build-embedded.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	hostlocal "github.com/containernetworking/plugins/plugins/ipam/host-local"
	"github.com/containernetworking/plugins/plugins/main/bridge"
	"github.com/containernetworking/plugins/plugins/main/loopback"
	"github.com/containernetworking/plugins/plugins/meta/portmap"
	"github.com/moby/sys/reexec"
)

func main() {
	os.Args[0] = filepath.Base(os.Args[0])
	reexec.Register("bridge", bridge.Main)
	reexec.Register("host-local", hostlocal.Main)
	reexec.Register("loopback", loopback.Main)
	reexec.Register("portmap", portmap.Main)
	if !reexec.Init() {
		_, _ = fmt.Fprintln(os.Stderr, bv.BuildString("plugins"))
		_, _ = fmt.Fprintf(os.Stderr, "CNI protocol versions supported: %s\n", strings.Join(version.All.SupportedVersions(), ", "))
	}
}
