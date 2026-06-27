package processmanager

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/containerexec"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

type execCallbackWrapper struct {
	inner          ExecSessionCallbackInterface
	onErrorCleanup func()
	once           sync.Once
}

func (w *execCallbackWrapper) GetStdinReader() io.Reader  { return w.inner.GetStdinReader() }
func (w *execCallbackWrapper) GetStdoutWriter() io.Writer { return w.inner.GetStdoutWriter() }
func (w *execCallbackWrapper) GetStderrWriter() io.Writer { return w.inner.GetStderrWriter() }
func (w *execCallbackWrapper) IsRunning() bool            { return w.inner.IsRunning() }

func (w *execCallbackWrapper) OnComplete() {
	w.inner.OnComplete()
}

func (w *execCallbackWrapper) OnError(err error) {
	w.once.Do(w.onErrorCleanup)
	w.inner.OnError(err)
}

func wrapExecCallback(callback ExecSessionCallbackInterface, cleanup func()) ExecSessionCallbackInterface {
	if callback == nil {
		return nil
	}
	if cleanup == nil {
		return callback
	}
	return &execCallbackWrapper{inner: callback, onErrorCleanup: cleanup}
}

func (pm *ProcessManager) ensureExecRegistry() *ExecSessionRegistry {
	if pm.execRegistry == nil {
		pm.execRegistry = NewExecSessionRegistry()
	}
	return pm.execRegistry
}

// CreateControllerExecSession registers a controller exec session, starts the shell, and blocks until running or timeout.
func (pm *ProcessManager) CreateControllerExecSession(sessionID, msUUID string, command []string, callback ExecSessionCallbackInterface) error {
	return pm.createRegistryExecSession(sessionID, msUUID, ExecOwnerController, runtimeExecIDController, command, callback, true)
}

// CreateLocalExecSession registers a local EdgeletAPI exec session with the local runtime id pattern.
// Interactive shells wait for the process to be running before POST returns; one-shot commands
// (ms exec <id> -- cmd) start I/O asynchronously local exec path.
func (pm *ProcessManager) CreateLocalExecSession(localSessionID, msUUID string, command []string, callback ExecSessionCallbackInterface) error {
	syncStart := containerexec.IsInteractiveShellCommand(command)
	return pm.createRegistryExecSession(localSessionID, msUUID, ExecOwnerLocal, runtimeExecIDLocal, command, callback, syncStart)
}

type runtimeExecIDBuilder func(containerID, sessionID string) string

func (pm *ProcessManager) createRegistryExecSession(
	sessionID, msUUID string,
	owner ExecOwner,
	buildRuntimeExecID runtimeExecIDBuilder,
	command []string,
	callback ExecSessionCallbackInterface,
	syncStart bool,
) error {
	sessionID = strings.TrimSpace(sessionID)
	msUUID = strings.TrimSpace(msUUID)
	if sessionID == "" {
		return errors.New("exec session id is required")
	}
	if msUUID == "" {
		return errors.New("microservice uuid is required")
	}
	if pm.engine == nil || pm.containerManager == nil {
		return errors.New("process manager engine is not initialized")
	}

	container, err := pm.containerManager.GetContainerForMicroservice(msUUID)
	if err != nil {
		return fmt.Errorf("failed to get container: %w", err)
	}
	if container == nil {
		return fmt.Errorf("container not found for microservice: %s", msUUID)
	}

	execIDHint := buildRuntimeExecID(container.ID, sessionID)
	registry := pm.ensureExecRegistry()
	if err := registry.Register(&ExecSessionRecord{
		SessionID:     sessionID,
		MSUUID:        msUUID,
		ContainerID:   container.ID,
		RuntimeExecID: execIDHint,
		Owner:         owner,
	}); err != nil {
		return err
	}

	releaseOnFailure := func() {
		registry.Remove(sessionID)
	}
	wrapped := wrapExecCallback(callback, releaseOnFailure)

	engineExecID, err := pm.prepareAndCreateExec(container.ID, execIDHint, command)
	if err != nil {
		releaseOnFailure()
		return err
	}
	registry.SetRuntimeExecID(sessionID, engineExecID)

	pm.launchExecSessionIO(engineExecID, wrapped)

	if !syncStart {
		return nil
	}

	if err := pm.waitExecSessionRunning(engineExecID, execStartGateDuration()); err != nil {
		_ = pm.engine.StopExecSession(engineExecID)
		releaseOnFailure()
		return err
	}
	registry.MarkStarted(sessionID)
	pm.logger.Infof("Exec session started: session=%s runtime=%s ms=%s owner=%s", sessionID, engineExecID, msUUID, owner)
	return nil
}

// ReleaseExecSession stops the runtime exec and removes the registry entry when owner matches.
func (pm *ProcessManager) ReleaseExecSession(sessionID string, owner ExecOwner) error {
	sessionID = strings.TrimSpace(sessionID)
	registry := pm.ensureExecRegistry()
	rec, err := registry.Release(sessionID, owner)
	if err != nil {
		return err
	}
	if pm.engine == nil {
		return nil
	}
	if stopErr := pm.engine.StopExecSession(rec.RuntimeExecID); stopErr != nil {
		pm.logger.Warnf("StopExecSession session=%s runtime=%s: %v", sessionID, rec.RuntimeExecID, stopErr)
	}
	return nil
}

// GetSession returns the registry record for attach/status lookups.
func (pm *ProcessManager) GetSession(sessionID string) (*ExecSessionRecord, bool) {
	return pm.ensureExecRegistry().Get(sessionID)
}

// ListControllerSessionsForMS lists controller-owned sessions for status reporting (Plan 23-3).
func (pm *ProcessManager) ListControllerSessionsForMS(msUUID string) []ExecSessionRecord {
	return pm.ensureExecRegistry().ListControllerSessionsForMS(msUUID)
}

// StopAllInteractiveForMicroservice stops all controller and local interactive exec sessions for one MS.
// Healthcheck exec paths are excluded — they never enter the registry.
func (pm *ProcessManager) StopAllInteractiveForMicroservice(msUUID string) {
	msUUID = strings.TrimSpace(msUUID)
	if msUUID == "" || pm.engine == nil {
		return
	}
	sessions := pm.ensureExecRegistry().ListInteractiveForMS(msUUID)
	for _, rec := range sessions {
		if err := pm.engine.StopExecSession(rec.RuntimeExecID); err != nil {
			pm.logger.Warnf("stop interactive exec ms=%s session=%s runtime=%s: %v", msUUID, rec.SessionID, rec.RuntimeExecID, err)
		}
		pm.ensureExecRegistry().Remove(rec.SessionID)
	}
}

func (pm *ProcessManager) prepareAndCreateExec(containerID, execIDHint string, command []string) (string, error) {
	if sweeper, ok := pm.engine.(engine.ExecOrphanSweeper); ok {
		keep := pm.ensureExecRegistry().RuntimeExecIDsForContainer(containerID)
		if err := sweeper.SweepOrphanExecSessions(containerID, keep); err != nil {
			pm.logger.Warnf("orphan exec sweep container=%s: %v", containerID, err)
		}
	}
	engineExecID, err := pm.engine.CreateExecSession(containerID, execIDHint, command)
	if err != nil {
		return "", fmt.Errorf("failed to create exec session: %w", err)
	}
	engineExecID = strings.TrimSpace(engineExecID)
	if engineExecID == "" {
		return "", errors.New("engine returned empty exec session id")
	}
	return engineExecID, nil
}

func (pm *ProcessManager) launchExecSessionIO(runtimeExecID string, callback ExecSessionCallbackInterface) {
	pm.engineMu.RLock()
	eng := pm.engine
	pm.engineMu.RUnlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogError(ProcessManagerModuleName, "Panic recovered", fmt.Errorf("%v", r))
			}
		}()
		if callback == nil || eng == nil {
			return
		}
		if err := eng.StartExecSession(
			runtimeExecID,
			callback.GetStdinReader(),
			callback.GetStdoutWriter(),
			callback.GetStderrWriter(),
		); err != nil {
			pm.logger.Errorf("Exec session %s I/O error: %v", runtimeExecID, err)
			callback.OnError(err)
		} else {
			callback.OnComplete()
		}
	}()
}

func (pm *ProcessManager) waitExecSessionRunning(runtimeExecID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := pm.engine.GetExecSessionStatus(runtimeExecID)
		if err != nil {
			return err
		}
		if running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%w after %s", ErrExecStartTimeout, timeout)
}
