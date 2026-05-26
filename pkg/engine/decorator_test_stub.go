package engine

import (
	"context"
	"io"

	"github.com/datasance/edgelet/internal/models"
)

// noopEngine is a minimal ContainerEngine stub for decorator tests.
type noopEngine struct{}

func (noopEngine) Init(EngineConfig) error                                      { return nil }
func (noopEngine) Close() error                                                 { return nil }
func (noopEngine) GetContainer(string) (*Container, error)                      { return nil, nil }
func (noopEngine) GetContainerByID(string) (*Container, error)                  { return nil, nil }
func (noopEngine) GetContainerSandboxID(string) (string, error)                 { return "", nil }
func (noopEngine) GetRunningContainers() ([]Container, error)                   { return nil, nil }
func (noopEngine) GetAllContainers() ([]Container, error)                       { return nil, nil }
func (noopEngine) CreateContainer(*models.Microservice, string) (string, error) { return "", nil }
func (noopEngine) StartContainer(string) error                                  { return nil }
func (noopEngine) StopContainer(string) error                                   { return nil }
func (noopEngine) KillContainer(string) error                                   { return nil }
func (noopEngine) RemoveContainer(string, bool) error                           { return nil }
func (noopEngine) PullImage(string, *models.Registry, *PullImageOptions) error  { return nil }
func (noopEngine) FindLocalImage(string) (bool, error)                          { return false, nil }
func (noopEngine) RemoveImage(string) error                                     { return nil }
func (noopEngine) PruneImages() error                                           { return nil }
func (noopEngine) ListImages(context.Context) ([]ImageInfo, error)              { return nil, nil }
func (noopEngine) LoadImageFromPath(context.Context, string) ([]LoadedImage, error) {
	return nil, nil
}
func (noopEngine) DeleteImage(context.Context, string) error { return nil }
func (noopEngine) PruneDangling(context.Context) (*ImagePruneReport, error) {
	return nil, nil
}
func (noopEngine) PruneContainers(context.Context) (*ContainerPruneReport, error) {
	return nil, nil
}
func (noopEngine) PruneVolumes(context.Context) (*VolumePruneReport, error) { return nil, nil }
func (noopEngine) GetContainerStatus(string, string) (*models.MicroserviceStatus, error) {
	return nil, nil
}
func (noopEngine) GetContainerStats(string) (*ContainerStats, error) { return nil, nil }
func (noopEngine) GetContainerIPAddress(string) (string, error)      { return "", nil }
func (noopEngine) GetContainerStartedAt(string) (int64, error)       { return 0, nil }
func (noopEngine) InspectContainerRaw(string) (map[string]interface{}, error) {
	return nil, nil
}
func (noopEngine) TailContainerLogs(string, string, string, LogTailHandler, *TailConfig) error {
	return nil
}
func (noopEngine) AreMicroserviceAndContainerEqual(string, *models.Microservice) bool { return false }
func (noopEngine) EnsureNetwork(string) error                                         { return nil }
func (noopEngine) CreateExecSession(string, []string) (string, error)                 { return "", nil }
func (noopEngine) StartExecSession(string, io.Reader, io.Writer, io.Writer) error     { return nil }
func (noopEngine) GetExecSessionStatus(string) (bool, error)                          { return false, nil }
func (noopEngine) GetExecSessionExitCode(string) (int, error)                         { return 0, nil }
func (noopEngine) ResizeExecSession(string, uint32, uint32) error                     { return nil }
func (noopEngine) StopExecSession(string) error                                       { return nil }
func (noopEngine) GetContainerMicroserviceUUID(Container) string                      { return "" }
func (noopEngine) GetContainerName(Container) string                                  { return "" }

var _ ContainerEngine = noopEngine{}
