package constants

// Container engine type names used in config.yaml containerEngine field.
const (
	EngineDocker = "docker"
	EnginePodman = "podman"
	EngineIofog  = "iofog"
)

// iofog embedded containerd — split across three path roots following Linux FHS
//
// /var/lib/iofog-agent/            — user data (diskDirectory config field)
//
//	diskConsumptionLimit applies only here
//
// /var/lib/iofog-agent-containerd/ — containerd images, snapshots, layers
//
//	intentionally outside diskDirectory so image storage
//	does not count against the user's disk limit
//
// /run/iofog-agent/                — ephemeral runtime files, cleared on reboot
const (
	// User data directory — matches default diskDirectory config value.
	IofogDataDir = "/var/lib/iofog-agent"

	// Containerd persistent data root — separate from IofogDataDir so that
	// image layers and snapshots are not counted by diskConsumptionLimit.
	IofogContainerdLibDir     = "/var/lib/iofog-agent-containerd"
	IofogContainerdRootDir    = "/var/lib/iofog-agent-containerd/root"
	IofogContainerdStateDir   = "/var/lib/iofog-agent-containerd/state"
	IofogContainerdBinDir     = "/var/lib/iofog-agent-containerd/bin"
	IofogCNIPluginsDir        = "/var/lib/iofog-agent-containerd/cni/plugins"
	IofogCNIConfDir           = "/var/lib/iofog-agent-containerd/cni/conf"
	IofogManagedCNIConfDir    = "/var/lib/iofog-agent-containerd/cni/conf/managed"
	IofogLocalCNIConfDir      = "/var/lib/iofog-agent-containerd/cni/conf/local"
	IofogContainerdImagesDir  = "/var/lib/iofog-agent-containerd/images"
	IofogManagedCNIConfigName = "10-iofog.conflist"
	IofogLocalCNIConfigName   = "11-iofog-local.conflist"
	IofogCNIConfigFile        = "/var/lib/iofog-agent-containerd/cni/conf/managed/10-iofog.conflist"
	IofogLocalCNIConfigFile   = "/var/lib/iofog-agent-containerd/cni/conf/local/11-iofog-local.conflist"
	IofogContainerdConfigFile = "/var/lib/iofog-agent-containerd/config.toml"

	// Ephemeral runtime directory — lives on tmpfs on systemd hosts, cleared on reboot.
	IofogRunDir           = "/run/iofog-agent"
	IofogContainerdSocket = "/run/iofog-agent/containerd.sock"

	// Standard system CNI config directory — symlink target for CNI config.
	DefaultSystemCNIConfDir = "/etc/cni/net.d"
	DefaultCNIConfigName    = IofogManagedCNIConfigName

	// iofog bridge network for containers managed by the embedded engine.
	// Uses 172.18.0.0/16 to avoid conflict with Docker's default 172.17.0.0/16.
	IofogBridgeName  = "iofog0"
	IofogBridgeCIDR  = "172.18.0.0/16"
	IofogNetworkName = "iofog"

	// Dedicated local-workload network.
	IofogLocalBridgeName  = "iofog-local0"
	IofogLocalBridgeCIDR  = "172.19.0.0/16"
	IofogLocalNetworkName = "iofog-local"

	// Containerd namespace used for all iofog-managed containers.
	IofogContainerdNamespace = "k8s.io"

	// Sandbox (pause) image for CRI podsandbox. portainer/pause is lightweight and multi-arch (incl. riscv64).
	IofogSandboxImage = "portainer/pause:latest"

	// PodmanDefaultDockerURL is the default socket URL for containerEngine podman.
	PodmanDefaultDockerURL = "unix:///run/podman/podman.sock"
)

// IofogEngineDockerURL returns the required dockerUrl when using embedded containerd (iofog engine).
func IofogEngineDockerURL() string {
	return "unix://" + IofogContainerdSocket
}
