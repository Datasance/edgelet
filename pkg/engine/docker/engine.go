// Package docker implements the ContainerEngine interface backed by the Docker daemon.
// It wraps the existing pkg/docker client so all Docker-specific logic stays there;
// this layer only adapts types to the engine-agnostic interface.
package docker

import (
	"context"
	"io"

	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/pkg/docker"
	"github.com/eclipse-iofog/agent/pkg/engine"
)

// Engine implements engine.ContainerEngine using the Docker daemon.
type Engine struct {
	client *docker.Client
}

// New returns a new Docker engine. Call Init() before use.
func New() *Engine {
	return &Engine{client: docker.GetInstance()}
}

// Init dials the Docker daemon socket.
func (e *Engine) Init(cfg engine.EngineConfig) error {
	return e.client.Init(cfg.SocketURL, cfg.APIVersion)
}

// Close releases the Docker client.
func (e *Engine) Close() error {
	return e.client.Close()
}

// --- Container lifecycle ---

func (e *Engine) GetContainer(microserviceUUID string) (*engine.Container, error) {
	c, err := e.client.GetContainer(microserviceUUID)
	if err != nil || c == nil {
		return nil, err
	}
	return adaptContainer(*c), nil
}

func (e *Engine) GetContainerByID(containerID string) (*engine.Container, error) {
	c, err := e.client.GetContainerByID(containerID)
	if err != nil || c == nil {
		return nil, err
	}
	return adaptContainer(*c), nil
}

func (e *Engine) GetContainerSandboxID(_ string) (string, error) {
	return "", nil // Docker has no sandbox concept
}

func (e *Engine) GetRunningContainers() ([]engine.Container, error) {
	cs, err := e.client.GetRunningContainers()
	if err != nil {
		return nil, err
	}
	return adaptContainers(cs), nil
}

func (e *Engine) GetAllContainers() ([]engine.Container, error) {
	cs, err := e.client.GetAllContainers()
	if err != nil {
		return nil, err
	}
	return adaptContainers(cs), nil
}

func (e *Engine) CreateContainer(ms *models.Microservice, hostname string) (string, error) {
	return e.client.CreateContainer(ms, hostname)
}

func (e *Engine) StartContainer(containerID string) error {
	return e.client.StartContainer(containerID)
}

func (e *Engine) StopContainer(containerID string) error {
	return e.client.StopContainer(containerID)
}

func (e *Engine) RemoveContainer(containerID string, removeVolumes bool) error {
	return e.client.RemoveContainer(containerID, removeVolumes)
}

// --- Image management ---

func (e *Engine) PullImage(imageRef string, registry *models.Registry, opts *engine.PullImageOptions) error {
	var cb func(float32)
	if opts != nil {
		cb = opts.ProgressCallback
	}
	return e.client.PullImage(imageRef, "", "", registry, cb)
}

func (e *Engine) FindLocalImage(imageRef string) (bool, error) {
	return e.client.FindLocalImage(imageRef)
}

func (e *Engine) RemoveImage(imageRef string) error {
	return e.client.RemoveImage(imageRef)
}

func (e *Engine) PruneImages() error {
	_, err := e.client.DockerPrune()
	return err
}

func (e *Engine) ListImages(_ context.Context) ([]engine.ImageInfo, error) {
	summaries, err := e.client.GetImages()
	if err != nil {
		return nil, err
	}
	result := make([]engine.ImageInfo, 0, len(summaries))
	for _, s := range summaries {
		result = append(result, engine.ImageInfo{
			ID:       s.ID,
			RepoTags: s.RepoTags,
		})
	}
	return result, nil
}

func (e *Engine) DeleteImage(_ context.Context, nameOrID string) error {
	return e.client.RemoveImage(nameOrID)
}

// PruneDangling removes only untagged images not referenced by any container.
// Matches Java DockerPruningManager.pruneAgent() which calls docker system prune (dangling only).
func (e *Engine) PruneDangling(_ context.Context) error {
	_, err := e.client.DockerPrune()
	return err
}

// --- Inspection / stats ---

func (e *Engine) GetContainerStatus(containerID, microserviceUUID string) (*models.MicroserviceStatus, error) {
	return e.client.GetMicroserviceStatus(containerID, microserviceUUID)
}

func (e *Engine) GetContainerStats(containerID string) (*engine.ContainerStats, error) {
	s, err := e.client.GetContainerStats(containerID)
	if err != nil {
		return nil, err
	}
	return &engine.ContainerStats{
		CPUUsage:    s.CPUUsage,
		MemoryUsage: s.MemoryUsage,
	}, nil
}

func (e *Engine) GetContainerIPAddress(containerID string) (string, error) {
	return e.client.GetContainerIPAddress(containerID)
}

func (e *Engine) GetContainerStartedAt(containerID string) (int64, error) {
	return e.client.GetContainerStartedAt(containerID)
}

// --- Log streaming ---

func (e *Engine) TailContainerLogs(containerID, sessionID, microserviceUUID string, handler engine.LogTailHandler, cfg *engine.TailConfig) error {
	dockerCfg := &docker.TailConfig{
		Follow: cfg.Follow,
		Lines:  cfg.Lines,
		Since:  cfg.Since,
		Until:  cfg.Until,
	}
	return e.client.TailContainerLogs(containerID, sessionID, microserviceUUID, &logHandlerAdapter{h: handler}, dockerCfg)
}

// --- Configuration drift ---

func (e *Engine) AreMicroserviceAndContainerEqual(containerID string, ms *models.Microservice) bool {
	return e.client.AreMicroserviceAndContainerEqual(containerID, ms)
}

// --- Network ---

func (e *Engine) EnsureNetwork(_ string) error {
	// The Docker client always ensures the "iofog" bridge network exists on Init.
	return nil
}

// --- Exec ---

func (e *Engine) CreateExecSession(containerID string, cmd []string) (string, error) {
	return e.client.CreateExecSession(containerID, cmd)
}

// StartExecSession attaches stdin/stdout/stderr to a previously created Docker exec session
// and starts it. The call blocks until the process exits or an error occurs.
func (e *Engine) StartExecSession(execID string, stdin io.Reader, stdout, stderr io.Writer) error {
	return e.client.StartExecSession(execID, stdin, stdout, stderr)
}

// StopExecSession is a no-op for Docker; exec lifecycle is managed by the daemon.
func (e *Engine) StopExecSession(_ string) error {
	return nil
}

// GetExecSessionStatus reports whether the Docker exec process is still running.
func (e *Engine) GetExecSessionStatus(execID string) (bool, error) {
	info, err := e.client.GetExecSessionStatus(execID)
	if err != nil {
		return false, err
	}
	return info.Running, nil
}

// --- Helpers ---

func (e *Engine) GetContainerMicroserviceUUID(cont engine.Container) string {
	return e.client.GetContainerMicroserviceUUID(docker.Container{
		ID:     cont.ID,
		Names:  cont.Names,
		Image:  cont.Image,
		Status: cont.Status,
		State:  cont.State,
		Labels: cont.Labels,
	})
}

func (e *Engine) GetContainerName(cont engine.Container) string {
	return e.client.GetContainerName(docker.Container{
		ID:     cont.ID,
		Names:  cont.Names,
		Image:  cont.Image,
		Status: cont.Status,
		State:  cont.State,
		Labels: cont.Labels,
	})
}

// --- Type adapters ---

func adaptContainer(c docker.Container) *engine.Container {
	return &engine.Container{
		ID:     c.ID,
		Names:  c.Names,
		Image:  c.Image,
		Status: c.Status,
		State:  c.State,
		Labels: c.Labels,
	}
}

func adaptContainers(cs []docker.Container) []engine.Container {
	out := make([]engine.Container, len(cs))
	for i, c := range cs {
		out[i] = engine.Container{
			ID:     c.ID,
			Names:  c.Names,
			Image:  c.Image,
			Status: c.Status,
			State:  c.State,
			Labels: c.Labels,
		}
	}
	return out
}

// logHandlerAdapter bridges engine.LogTailHandler to docker.LogTailHandler.
type logHandlerAdapter struct {
	h engine.LogTailHandler
}

func (a *logHandlerAdapter) OnLogLine(sessionID, microserviceUUID string, line []byte, st docker.StreamType) {
	var est engine.StreamType
	if st == docker.STDERR {
		est = engine.Stderr
	}
	a.h.OnLogLine(sessionID, microserviceUUID, line, est)
}

func (a *logHandlerAdapter) OnComplete(sessionID string) { a.h.OnComplete(sessionID) }
func (a *logHandlerAdapter) OnError(sessionID string, err error) {
	a.h.OnError(sessionID, err)
}
