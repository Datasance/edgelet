package engine

import (
	"context"
	"io"
	"time"

	"github.com/datasance/edgelet/internal/models"
)

// ImageInfo is the engine-agnostic representation of a local container image.
type ImageInfo struct {
	ID         string    // content-addressable digest or engine-specific ID
	RepoTags   []string  // list of "name:tag" references; may be empty for dangling images
	ShortID    string    // short display id when available
	Repository string    // repository part of primary tag
	Tag        string    // tag part of primary tag
	Digest     string    // primary digest when available
	CreatedAt  time.Time // image creation timestamp when available
	SizeBytes  int64     // image size in bytes when available
	Engine     string    // engine that emitted this record (docker|podman|iofog)
}

// LoadedImage is a normalized result entry returned by load/import operations.
type LoadedImage struct {
	Name string
	ID   string
}

// ImagePruneReport captures normalized dangling-prune results.
type ImagePruneReport struct {
	Deleted             []string
	DeletedCount        int
	SpaceReclaimedBytes int64
}

// ContainerPruneReport captures normalized container-prune results.
type ContainerPruneReport struct {
	Deleted      []string
	DeletedCount int
}

// VolumePruneReport captures normalized volume-prune results.
type VolumePruneReport struct {
	Deleted             []string
	DeletedCount        int
	SpaceReclaimedBytes int64
}

// ContainerEngine is the abstraction over Docker, Podman, and the embedded
// iofog containerd engine. ProcessManager uses only this interface so that the
// underlying runtime is fully interchangeable via the containerEngine config field.
type ContainerEngine interface {
	// Init initializes the engine. For docker/podman this dials the daemon socket;
	// for iofog it extracts embedded binaries and starts containerd in-process.
	Init(cfg EngineConfig) error

	// Container lifecycle
	GetContainer(microserviceUUID string) (*Container, error)
	// GetContainerByID returns a container by its engine-assigned ID. Used when the caller
	// already has the ID (e.g. from container_state DB). Returns nil if not found.
	GetContainerByID(containerID string) (*Container, error)
	// GetContainerSandboxID returns the sandbox (pause) container ID for the given workload
	// container. Only meaningful for iofog/CRI; Docker/Podman return "".
	GetContainerSandboxID(containerID string) (string, error)
	GetRunningContainers() ([]Container, error)
	GetAllContainers() ([]Container, error)
	CreateContainer(ms *models.Microservice, hostname string) (string, error)
	StartContainer(containerID string) error
	StopContainer(containerID string) error
	KillContainer(containerID string) error
	RemoveContainer(containerID string, removeVolumes bool) error

	// Image management
	PullImage(imageRef string, registry *models.Registry, opts *PullImageOptions) error
	FindLocalImage(imageRef string) (bool, error)
	RemoveImage(imageRef string) error
	// PruneImages prunes images not referenced by running containers.
	// Deprecated: prefer the unified ListImages/DeleteImage path via the pruning manager.
	PruneImages() error
	// ListImages returns all locally available images.
	ListImages(ctx context.Context) ([]ImageInfo, error)
	// LoadImageFromPath imports an image archive from daemon-local filesystem path.
	LoadImageFromPath(ctx context.Context, archivePath string) ([]LoadedImage, error)
	// DeleteImage removes an image by its ID or name:tag reference.
	DeleteImage(ctx context.Context, nameOrID string) error
	// PruneDangling removes only untagged images not referenced by any container.
	PruneDangling(ctx context.Context) (*ImagePruneReport, error)
	// PruneContainers removes stopped/orphaned containers that are safe to delete.
	PruneContainers(ctx context.Context) (*ContainerPruneReport, error)
	// PruneVolumes removes unused/orphaned volume artifacts.
	PruneVolumes(ctx context.Context) (*VolumePruneReport, error)
	// RemoveNamedVolume deletes a named volume when it exists.
	RemoveNamedVolume(ctx context.Context, name string) error

	// Inspection / stats
	GetContainerStatus(containerID, microserviceUUID string) (*models.MicroserviceStatus, error)
	GetContainerStats(containerID string) (*ContainerStats, error)
	GetContainerIPAddress(containerID string) (string, error)
	GetContainerStartedAt(containerID string) (int64, error)
	InspectContainerRaw(containerID string) (map[string]interface{}, error)

	// Log streaming
	TailContainerLogs(containerID, sessionID, microserviceUUID string, handler LogTailHandler, cfg *TailConfig) error

	// Configuration drift detection
	AreMicroserviceAndContainerEqual(containerID string, ms *models.Microservice) bool

	// Network
	EnsureNetwork(name string) error

	// Exec session
	// CreateExecSession registers an exec spec for the given container and returns an execID.
	// The process is NOT started yet; call StartExecSession to attach I/O and launch it.
	CreateExecSession(containerID string, cmd []string) (string, error)
	// StartExecSession attaches the given stdin/stdout/stderr pipes to the exec process
	// identified by execID and starts it. Must be called after CreateExecSession.
	StartExecSession(execID string, stdin io.Reader, stdout, stderr io.Writer) error
	// GetExecSessionStatus reports whether the exec process identified by execID is still running.
	GetExecSessionStatus(execID string) (running bool, err error)
	// GetExecSessionExitCode returns exit status when the exec process has completed.
	GetExecSessionExitCode(execID string) (exitCode int, err error)
	// ResizeExecSession resizes an interactive TTY exec session.
	ResizeExecSession(execID string, cols, rows uint32) error
	// StopExecSession kills and deregisters the exec process. For iofog engine this is required
	// when the controller closes the WebSocket so the exec ID can be reused; Docker/Podman no-op.
	StopExecSession(execID string) error

	// Helpers used by ProcessManager
	GetContainerMicroserviceUUID(cont Container) string
	GetContainerName(cont Container) string

	// Close releases any resources held by the engine.
	Close() error
}

// PullImageOptions allows optional progress reporting. If nil, no progress is reported.
type PullImageOptions struct {
	Out              io.Writer
	ProgressCallback func(float32)
	// Platform is an optional OCI platform selector (e.g. linux/amd64).
	// When set, engines should map it to their native pull option.
	Platform string
}

// EngineConfig holds the configuration passed to ContainerEngine.Init().
// For docker/podman the SocketURL and APIVersion fields are used.
// For iofog the SocketURL is always overridden by the EdgeletContainerdSocket constant.
type EngineConfig struct {
	SocketURL  string // e.g. "unix:///var/run/docker.sock"
	APIVersion string // Docker API version negotiation (empty = auto)
	LogDir     string // directory for container log files (iofog engine only)
}

// Container is the engine-agnostic representation of a running or stopped container.
// It mirrors the fields ProcessManager actually uses, so docker.Container and any
// future engine-specific types can all be mapped to this common struct.
type Container struct {
	ID     string
	Names  []string
	Image  string
	Status string
	State  string
	Labels map[string]string
}

// ContainerStats holds per-container CPU and memory statistics.
type ContainerStats struct {
	CPUUsage    float32
	MemoryUsage int64
}

// TailConfig configures how container logs are tailed.
type TailConfig struct {
	Follow bool
	Lines  int
	Since  string
	Until  string
}

// StreamType distinguishes stdout from stderr in log lines.
type StreamType int

const (
	Stdout StreamType = iota
	Stderr
)

// LogTailHandler is the callback interface used by TailContainerLogs.
type LogTailHandler interface {
	OnLogLine(sessionID, microserviceUUID string, line []byte, st StreamType)
	OnComplete(sessionID string)
	OnError(sessionID string, err error)
}
