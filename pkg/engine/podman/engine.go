// Package podman implements the ContainerEngine interface backed by the Podman daemon.
// Podman exposes a Docker-compatible API, so this engine is a thin wrapper over the
// Docker engine adapter pointed at the Podman socket.
package podman

import (
	"context"
	"io"
	"os"

	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/pkg/engine"
	dockerengine "github.com/eclipse-iofog/agent/pkg/engine/docker"
)

const (
	// DefaultPodmanSocket is the default Podman socket path for the root user.
	DefaultPodmanSocket = "unix:///run/podman/podman.sock"
	// DefaultPodmanUserSocket is the default Podman socket for rootless installs.
	DefaultPodmanUserSocket = "unix:///run/user/0/podman/podman.sock"
)

// Engine implements engine.ContainerEngine using the Podman daemon via its
// Docker-compatible socket API.
type Engine struct {
	inner *dockerengine.Engine
}

// New returns a new Podman engine. Call Init() before use.
func New() *Engine {
	return &Engine{inner: dockerengine.New()}
}

// Init connects to the Podman socket. If cfg.SocketURL is empty it auto-detects
// the Podman socket by trying the root socket and the rootless socket in order.
func (e *Engine) Init(cfg engine.EngineConfig) error {
	if cfg.SocketURL == "" {
		cfg.SocketURL = detectPodmanSocket()
	}
	return e.inner.Init(cfg)
}

func (e *Engine) Close() error                                     { return e.inner.Close() }
func (e *Engine) GetContainer(u string) (*engine.Container, error) { return e.inner.GetContainer(u) }
func (e *Engine) GetContainerByID(id string) (*engine.Container, error) {
	return e.inner.GetContainerByID(id)
}
func (e *Engine) GetContainerSandboxID(id string) (string, error) {
	return e.inner.GetContainerSandboxID(id)
}
func (e *Engine) GetRunningContainers() ([]engine.Container, error) {
	return e.inner.GetRunningContainers()
}
func (e *Engine) GetAllContainers() ([]engine.Container, error) { return e.inner.GetAllContainers() }
func (e *Engine) CreateContainer(ms *models.Microservice, h string) (string, error) {
	return e.inner.CreateContainer(ms, h)
}
func (e *Engine) StartContainer(id string) error          { return e.inner.StartContainer(id) }
func (e *Engine) StopContainer(id string) error           { return e.inner.StopContainer(id) }
func (e *Engine) KillContainer(id string) error           { return e.inner.KillContainer(id) }
func (e *Engine) RemoveContainer(id string, v bool) error { return e.inner.RemoveContainer(id, v) }
func (e *Engine) PullImage(ref string, reg *models.Registry, opts *engine.PullImageOptions) error {
	return e.inner.PullImage(ref, reg, opts)
}
func (e *Engine) FindLocalImage(ref string) (bool, error) { return e.inner.FindLocalImage(ref) }
func (e *Engine) RemoveImage(ref string) error            { return e.inner.RemoveImage(ref) }
func (e *Engine) PruneImages() error                      { return e.inner.PruneImages() }
func (e *Engine) GetContainerStatus(id, uuid string) (*models.MicroserviceStatus, error) {
	return e.inner.GetContainerStatus(id, uuid)
}
func (e *Engine) GetContainerStats(id string) (*engine.ContainerStats, error) {
	return e.inner.GetContainerStats(id)
}
func (e *Engine) GetContainerIPAddress(id string) (string, error) {
	return e.inner.GetContainerIPAddress(id)
}
func (e *Engine) GetContainerStartedAt(id string) (int64, error) {
	return e.inner.GetContainerStartedAt(id)
}
func (e *Engine) TailContainerLogs(id, sid, uuid string, h engine.LogTailHandler, cfg *engine.TailConfig) error {
	return e.inner.TailContainerLogs(id, sid, uuid, h, cfg)
}
func (e *Engine) AreMicroserviceAndContainerEqual(id string, ms *models.Microservice) bool {
	return e.inner.AreMicroserviceAndContainerEqual(id, ms)
}
func (e *Engine) EnsureNetwork(name string) error { return e.inner.EnsureNetwork(name) }
func (e *Engine) CreateExecSession(id string, cmd []string) (string, error) {
	return e.inner.CreateExecSession(id, cmd)
}
func (e *Engine) StartExecSession(execID string, stdin io.Reader, stdout, stderr io.Writer) error {
	return e.inner.StartExecSession(execID, stdin, stdout, stderr)
}
func (e *Engine) GetExecSessionStatus(execID string) (bool, error) {
	return e.inner.GetExecSessionStatus(execID)
}
func (e *Engine) GetExecSessionExitCode(execID string) (int, error) {
	return e.inner.GetExecSessionExitCode(execID)
}
func (e *Engine) ResizeExecSession(execID string, cols, rows uint32) error {
	return e.inner.ResizeExecSession(execID, cols, rows)
}
func (e *Engine) StopExecSession(execID string) error {
	return e.inner.StopExecSession(execID)
}
func (e *Engine) GetContainerMicroserviceUUID(c engine.Container) string {
	return e.inner.GetContainerMicroserviceUUID(c)
}
func (e *Engine) GetContainerName(c engine.Container) string { return e.inner.GetContainerName(c) }
func (e *Engine) ListImages(ctx context.Context) ([]engine.ImageInfo, error) {
	return e.inner.ListImages(ctx)
}
func (e *Engine) LoadImageFromPath(ctx context.Context, archivePath string) ([]engine.LoadedImage, error) {
	return e.inner.LoadImageFromPath(ctx, archivePath)
}
func (e *Engine) DeleteImage(ctx context.Context, nameOrID string) error {
	return e.inner.DeleteImage(ctx, nameOrID)
}
func (e *Engine) PruneDangling(ctx context.Context) (*engine.ImagePruneReport, error) {
	return e.inner.PruneDangling(ctx)
}
func (e *Engine) InspectContainerRaw(containerID string) (map[string]interface{}, error) {
	return e.inner.InspectContainerRaw(containerID)
}

// detectPodmanSocket returns the first Podman socket path that exists on disk.
func detectPodmanSocket() string {
	candidates := []string{
		"/run/podman/podman.sock",
		"/run/user/0/podman/podman.sock",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return "unix://" + p
		}
	}
	return DefaultPodmanSocket
}
