package processmanager

import (
	"context"
	"testing"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/statusreporter"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/internal/workloadmeta"
	"github.com/datasance/edgelet/pkg/engine"
)

type removeByIDTestEngine struct {
	engine.ContainerEngine
	container engine.Container
}

func (e *removeByIDTestEngine) GetContainerByID(id string) (*engine.Container, error) {
	if e.container.ID == id {
		c := e.container
		return &c, nil
	}
	return nil, nil
}

func (e *removeByIDTestEngine) StopContainer(string) error { return nil }

func (e *removeByIDTestEngine) RemoveContainer(string, bool) error { return nil }

func TestRemoveContainerByID_SetsDeletedForManagedMicroserviceUUID(t *testing.T) {
	openLocalReconcileTestDB(t)
	t.Cleanup(func() { statusreporter.GetInstance().ResetProcessManagerStatus() })

	statusreporter.GetInstance().ResetProcessManagerStatus()
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		pm.SetMicroservicesState("ms-by-id", models.MicroserviceStateRunning)
	})

	eng := &removeByIDTestEngine{
		container: engine.Container{
			ID: "container-1",
			Labels: map[string]string{
				workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
				workloadmeta.LabelMicroserviceUID: "ms-by-id",
				workloadmeta.LabelScope:           workloadmeta.ScopeManaged,
			},
		},
	}
	cm := NewContainerManager(eng, nil, "docker")
	cm.logger = logging.NewModuleLogger("test-container-manager")

	if err := cm.RemoveContainerByID(context.Background(), "container-1", false, false); err != nil {
		t.Fatalf("unexpected remove error: %v", err)
	}

	st := statusreporter.GetInstance().GetProcessManagerStatus().GetMicroserviceStatus("ms-by-id")
	if st == nil || st.Status != models.MicroserviceStateDeleted {
		t.Fatalf("expected deleted status after remove by container id, got %#v", st)
	}
}
