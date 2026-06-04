package processmanager

import (
	"errors"
	"testing"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/statusreporter"
	"github.com/datasance/edgelet/internal/store"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/internal/workloadmeta"
	"github.com/datasance/edgelet/pkg/engine"
)

type pruneTestMicroserviceManager struct {
	latest []*models.Microservice
}

func (m *pruneTestMicroserviceManager) GetLatestMicroservices() []*models.Microservice {
	return m.latest
}

func (m *pruneTestMicroserviceManager) GetCurrentMicroservices() []*models.Microservice { return nil }

func (m *pruneTestMicroserviceManager) FindLatestMicroserviceByUUID(uuid string) *models.Microservice {
	for _, ms := range m.latest {
		if ms != nil && ms.MicroserviceUUID == uuid {
			return ms
		}
	}
	return nil
}

func (m *pruneTestMicroserviceManager) GetRegistry(_ int) *models.Registry { return nil }

func (m *pruneTestMicroserviceManager) SetCurrentMicroservices(_ []*models.Microservice) {}

type pruneTestEngine struct {
	engine.ContainerEngine
	allContainers []engine.Container
	allErr        error
}

func (e *pruneTestEngine) GetAllContainers() ([]engine.Container, error) {
	if e.allErr != nil {
		return nil, e.allErr
	}
	return e.allContainers, nil
}

func (e *pruneTestEngine) GetContainerMicroserviceUUID(container engine.Container) string {
	return workloadmeta.MicroserviceUIDFromLabels(container.Labels)
}

func newPruneTestProcessManager(latest []*models.Microservice, eng engine.ContainerEngine) *ProcessManager {
	return &ProcessManager{
		logger:              logging.NewModuleLogger("test-process-manager-prune"),
		microserviceManager: &pruneTestMicroserviceManager{latest: latest},
		engine:              eng,
	}
}

func seedStatus(uuid string, state models.MicroserviceState) {
	sr := statusreporter.GetInstance()
	sr.ResetProcessManagerStatus()
	sr.UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		pm.SetMicroservicesState(uuid, state)
	})
}

func TestPruneStaleProcessManagerStatuses_PrunesOrphanManagedStatus(t *testing.T) {
	openLocalReconcileTestDB(t)
	t.Cleanup(func() { statusreporter.GetInstance().ResetProcessManagerStatus() })

	seedStatus("stale-managed", models.MicroserviceStateRunning)

	pm := newPruneTestProcessManager(nil, &pruneTestEngine{})
	pm.pruneStaleProcessManagerStatuses()

	if _, ok := statusreporter.GetInstance().GetProcessManagerStatus().MicroservicesStatus["stale-managed"]; ok {
		t.Fatalf("expected stale-managed status to be pruned")
	}
}

func TestPruneStaleProcessManagerStatuses_KeepsManagedLocalAndRuntimeOwned(t *testing.T) {
	openLocalReconcileTestDB(t)
	t.Cleanup(func() { statusreporter.GetInstance().ResetProcessManagerStatus() })

	sr := statusreporter.GetInstance()
	sr.ResetProcessManagerStatus()
	sr.UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		pm.SetMicroservicesState("managed-1", models.MicroserviceStateRunning)
		pm.SetMicroservicesState("local-1", models.MicroserviceStateRunning)
		pm.SetMicroservicesState("runtime-only-1", models.MicroserviceStateRunning)
	})

	local := &models.LocalDeployedMicroservice{
		LocalUUID:        "local-1",
		ApplicationName:  "edgelet",
		MicroserviceName: "router",
		SourceName:       "local-cli",
		ManifestYAML:     "kind: Microservice",
		ImageName:        "nginx:latest",
		State:            "running",
	}
	if err := store.GetInstance().UpsertLocalWorkload(local); err != nil {
		t.Fatalf("failed to seed local deployment: %v", err)
	}

	latest := []*models.Microservice{
		{MicroserviceUUID: "managed-1"},
	}
	eng := &pruneTestEngine{
		allContainers: []engine.Container{
			{
				ID: "runtime-only-container",
				Labels: map[string]string{
					workloadmeta.LabelMicroserviceUID: "runtime-only-1",
				},
			},
		},
	}
	pm := newPruneTestProcessManager(latest, eng)
	pm.pruneStaleProcessManagerStatuses()

	statuses := statusreporter.GetInstance().GetProcessManagerStatus().MicroservicesStatus
	if _, ok := statuses["managed-1"]; !ok {
		t.Fatalf("expected managed status to be kept")
	}
	if _, ok := statuses["local-1"]; !ok {
		t.Fatalf("expected local status to be kept")
	}
	if _, ok := statuses["runtime-only-1"]; !ok {
		t.Fatalf("expected runtime-only transitional status to be kept")
	}
}

func TestPruneStaleProcessManagerStatuses_FailOpenWhenLocalListFails(t *testing.T) {
	_ = store.GetInstance().Close()
	t.Cleanup(func() { statusreporter.GetInstance().ResetProcessManagerStatus() })

	seedStatus("stale-managed", models.MicroserviceStateRunning)

	pm := newPruneTestProcessManager(nil, &pruneTestEngine{})
	pm.pruneStaleProcessManagerStatuses()

	if _, ok := statusreporter.GetInstance().GetProcessManagerStatus().MicroservicesStatus["stale-managed"]; !ok {
		t.Fatalf("expected stale-managed status to remain when local list fails")
	}
}

func TestPruneStaleProcessManagerStatuses_KeepsDeletedOrphanForControllerReport(t *testing.T) {
	openLocalReconcileTestDB(t)
	t.Cleanup(func() { statusreporter.GetInstance().ResetProcessManagerStatus() })

	seedStatus("removed-ms", models.MicroserviceStateDeleted)

	pm := newPruneTestProcessManager(nil, &pruneTestEngine{})
	pm.pruneStaleProcessManagerStatuses()

	st, ok := statusreporter.GetInstance().GetProcessManagerStatus().MicroservicesStatus["removed-ms"]
	if !ok {
		t.Fatalf("expected deleted orphan status to remain for controller reporting")
	}
	if st == nil || st.Status != models.MicroserviceStateDeleted {
		t.Fatalf("expected deleted state to be preserved, got %#v", st)
	}
}

func TestPruneStaleProcessManagerStatuses_KeepsTerminalOrphans(t *testing.T) {
	openLocalReconcileTestDB(t)
	t.Cleanup(func() { statusreporter.GetInstance().ResetProcessManagerStatus() })

	sr := statusreporter.GetInstance()
	sr.ResetProcessManagerStatus()
	sr.UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		pm.SetMicroservicesState("ms-unknown", models.MicroserviceStateUnknown)
		pm.SetMicroservicesState("ms-deleting", models.MicroserviceStateDeleting)
		pm.SetMicroservicesState("ms-marked", models.MicroserviceStateMarkedForDeletion)
	})

	pm := newPruneTestProcessManager(nil, &pruneTestEngine{})
	pm.pruneStaleProcessManagerStatuses()

	statuses := sr.GetProcessManagerStatus().MicroservicesStatus
	for _, uuid := range []string{"ms-unknown", "ms-deleting", "ms-marked"} {
		if _, ok := statuses[uuid]; !ok {
			t.Fatalf("expected terminal status %q to remain after prune", uuid)
		}
	}
}

func TestPruneStaleProcessManagerStatuses_FailOpenWhenRuntimeListFails(t *testing.T) {
	openLocalReconcileTestDB(t)
	t.Cleanup(func() { statusreporter.GetInstance().ResetProcessManagerStatus() })

	seedStatus("stale-managed", models.MicroserviceStateRunning)

	pm := newPruneTestProcessManager(nil, &pruneTestEngine{allErr: errors.New("runtime unavailable")})
	pm.pruneStaleProcessManagerStatuses()

	if _, ok := statusreporter.GetInstance().GetProcessManagerStatus().MicroservicesStatus["stale-managed"]; !ok {
		t.Fatalf("expected stale-managed status to remain when runtime list fails")
	}
}
