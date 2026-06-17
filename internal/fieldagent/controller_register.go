package fieldagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/controlplane"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

const controllerRegisterInterval = 30 * time.Second

type controllerRegisterState struct {
	mu                    sync.Mutex
	succeeded             map[string]struct{}
	initialRebuildSkipped map[string]struct{}
}

func newControllerRegisterState() *controllerRegisterState {
	return &controllerRegisterState{
		succeeded:             make(map[string]struct{}),
		initialRebuildSkipped: make(map[string]struct{}),
	}
}

func (s *controllerRegisterState) markSucceeded(uuid string) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.succeeded[uuid] = struct{}{}
}

func (s *controllerRegisterState) isSucceeded(uuid string) bool {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.succeeded[uuid]
	return ok
}

func (s *controllerRegisterState) markInitialRebuildSkipped(uuid string) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initialRebuildSkipped == nil {
		s.initialRebuildSkipped = make(map[string]struct{})
	}
	s.initialRebuildSkipped[uuid] = struct{}{}
}

func (s *controllerRegisterState) isInitialRebuildSkipped(uuid string) bool {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.initialRebuildSkipped[uuid]
	return ok
}

func (s *controllerRegisterState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.succeeded = make(map[string]struct{})
	s.initialRebuildSkipped = make(map[string]struct{})
}

func (fa *FieldAgent) resetControllerRegisterState() {
	if fa.controllerRegister == nil {
		return
	}
	fa.controllerRegister.reset()
}

func (fa *FieldAgent) hydrateControllerRegisterState() {
	if fa.controllerRegister == nil {
		return
	}
	cp, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found || cp == nil {
		return
	}
	uuid := strings.TrimSpace(cp.ControllerUUID)
	if uuid == "" {
		return
	}
	if cp.ControllerRegistered {
		fa.controllerRegister.markSucceeded(uuid)
	}
	if cp.InitialRebuildSkipped {
		fa.controllerRegister.markInitialRebuildSkipped(uuid)
	}
}

func (fa *FieldAgent) persistControllerRegistered(uuid string) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return
	}
	cp, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found || cp == nil {
		return
	}
	if strings.TrimSpace(cp.ControllerUUID) != uuid {
		return
	}
	cp.ControllerRegistered = true
	if err := store.GetInstance().UpsertSystemControlPlane(cp); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("persist controller registered uuid=%s err=%v", uuid, err))
	}
}

func (fa *FieldAgent) persistControllerInitialRebuildSkipped(uuid string) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return
	}
	cp, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found || cp == nil {
		return
	}
	if strings.TrimSpace(cp.ControllerUUID) != uuid {
		return
	}
	cp.InitialRebuildSkipped = true
	if err := store.GetInstance().UpsertSystemControlPlane(cp); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("persist controller initial rebuild skipped uuid=%s err=%v", uuid, err))
	}
}

func (fa *FieldAgent) controllerRegisterWorker() {
	defer fa.wg.Done()

	timer := time.NewTimer(controllerRegisterInterval)
	defer timer.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-timer.C:
			fa.tryControllerRegister()
			timer.Reset(controllerRegisterInterval)
		}
	}
}

func (fa *FieldAgent) tryControllerRegister() {
	if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
		return
	}
	if fa.getAPIClient() == nil {
		return
	}

	cp, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found || cp == nil {
		return
	}
	cp.NormalizeDefaults()
	controllerUUID := strings.TrimSpace(cp.ControllerUUID)
	if controllerUUID == "" {
		return
	}
	if fa.controllerRegister != nil && fa.controllerRegister.isSucceeded(controllerUUID) {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(cp.DesiredState), "running") {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(cp.RuntimeState), "running") {
		return
	}
	if fa.processManager == nil {
		return
	}
	container, err := fa.processManager.GetContainerForMicroservice(controllerUUID)
	if err != nil || container == nil {
		logging.LogDebug(moduleName, fmt.Sprintf("controller register waiting for running container uuid=%s", controllerUUID))
		return
	}

	body, err := buildControllerRegisterBody(fa.config, cp)
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("controller register body build failed uuid=%s err=%v", controllerUUID, err))
		return
	}

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	apiClient := fa.getAPIClient()
	if apiClient == nil {
		cancel()
		return
	}
	result, reqErr := apiClient.Request(ctx, "controller/register", POST, nil, body)
	cancel()
	if reqErr != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("controller register request failed uuid=%s err=%v", controllerUUID, reqErr))
		return
	}

	respUUID, ok := result["uuid"].(string)
	if !ok || strings.TrimSpace(respUUID) == "" {
		respUUID = controllerUUID
	}
	if strings.TrimSpace(respUUID) != controllerUUID {
		logging.LogWarn(moduleName, fmt.Sprintf(
			"controller register uuid mismatch request=%s response=%s",
			controllerUUID,
			respUUID,
		))
		return
	}

	if fa.controllerRegister != nil {
		fa.controllerRegister.markSucceeded(controllerUUID)
	}
	fa.persistControllerRegistered(controllerUUID)
	logging.LogInfo(moduleName, fmt.Sprintf("controller microservice registered uuid=%s", controllerUUID))
}

func buildControllerRegisterBody(cfg *config.Config, cp *models.ControlPlaneDeployment) (map[string]any, error) {
	if cp == nil {
		return nil, errors.New("control plane deployment is nil")
	}
	doc, err := models.ParseControlPlaneManifest(cp.ManifestYAML)
	if err != nil {
		return nil, err
	}
	image := doc.ManifestControllerImage()
	ms, err := controlplane.BuildMicroserviceFromControlPlane(doc, cp.ControllerUUID, image)
	if err != nil {
		return nil, err
	}

	archID := config.ArchitectureCode(cfg.Arch)
	body := map[string]any{
		"uuid":       strings.TrimSpace(cp.ControllerUUID),
		"name":       "controller",
		"schedule":   0,
		"images":     []map[string]any{{"containerImage": image, "archId": archID}},
		"registryId": ms.RegistryID,
	}
	if len(ms.PortMappings) > 0 {
		body["ports"] = portMappingsToRegisterBody(ms.PortMappings)
	}
	if len(ms.VolumeMappings) > 0 {
		body["volumeMappings"] = volumeMappingsToRegisterBody(ms.VolumeMappings)
	}
	envMap, err := controlplane.BuildControllerEnv(doc, cp.ControllerUUID)
	if err != nil {
		return nil, err
	}
	body["env"] = envVarsToRegisterBody(envMap)
	if ms.Runtime != nil && strings.TrimSpace(*ms.Runtime) != "" {
		body["runtime"] = strings.TrimSpace(*ms.Runtime)
	}
	if ms.HostNetworkMode {
		body["hostNetworkMode"] = true
	}
	if ms.Config != nil {
		body["config"] = *ms.Config
	}
	return body, nil
}

func portMappingsToRegisterBody(mappings []*models.PortMapping) []map[string]any {
	out := make([]map[string]any, 0, len(mappings))
	for _, pm := range mappings {
		if pm == nil {
			continue
		}
		protocol := "tcp"
		if pm.UDP {
			protocol = "udp"
		}
		out = append(out, map[string]any{
			"internal": pm.Inside,
			"external": pm.Outside,
			"protocol": protocol,
		})
	}
	return out
}

func volumeMappingsToRegisterBody(mappings []*models.VolumeMapping) []map[string]any {
	out := make([]map[string]any, 0, len(mappings))
	for _, vm := range mappings {
		if vm == nil {
			continue
		}
		typeName := "bind"
		switch vm.Type {
		case models.VolumeMappingTypeVolume:
			typeName = "volume"
		case models.VolumeMappingTypeVolumeMount:
			typeName = "volumeMount"
		}
		out = append(out, map[string]any{
			"hostDestination":      vm.HostDestination,
			"containerDestination": vm.ContainerDestination,
			"accessMode":           vm.AccessMode,
			"type":                 typeName,
		})
	}
	return out
}

func envVarsToRegisterBody(env map[string]string) []map[string]any {
	out := make([]map[string]any, 0, len(env))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out = append(out, map[string]any{
			"key":   key,
			"value": value,
		})
	}
	return out
}
