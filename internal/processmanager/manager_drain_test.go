package processmanager

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/datasance/edgelet/internal/runtimeops"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/internal/workloadmeta"
	"github.com/datasance/edgelet/pkg/engine"
)

type drainTestEngine struct {
	engine.ContainerEngine

	mu          sync.Mutex
	running     map[string]engine.Container
	stopRemoves bool
	stopDelay   time.Duration

	stopCalls     int64
	activeStops   int64
	maxConcurrent int64
	stopErrByID   map[string]error
	killErrByID   map[string]error
}

func newDrainTestEngine(ids ...string) *drainTestEngine {
	running := make(map[string]engine.Container, len(ids))
	for _, id := range ids {
		running[id] = engine.Container{
			ID: id,
			Labels: map[string]string{
				workloadmeta.LabelMicroserviceUID: "ms-" + id,
			},
		}
	}
	return &drainTestEngine{
		running:     running,
		stopRemoves: true,
		stopErrByID: make(map[string]error),
		killErrByID: make(map[string]error),
	}
}

func (e *drainTestEngine) GetRunningContainers() ([]engine.Container, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	items := make([]engine.Container, 0, len(e.running))
	for _, c := range e.running {
		items = append(items, c)
	}
	return items, nil
}

func (e *drainTestEngine) GetContainerByID(id string) (*engine.Container, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	c, ok := e.running[id]
	if !ok {
		return nil, nil
	}
	item := c
	return &item, nil
}

func (e *drainTestEngine) GetContainerMicroserviceUUID(container engine.Container) string {
	return strings.TrimSpace(container.Labels[workloadmeta.LabelMicroserviceUID])
}

func (e *drainTestEngine) StopContainer(id string) error {
	atomic.AddInt64(&e.stopCalls, 1)
	active := atomic.AddInt64(&e.activeStops, 1)
	defer atomic.AddInt64(&e.activeStops, -1)
	for {
		current := atomic.LoadInt64(&e.maxConcurrent)
		if active <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&e.maxConcurrent, current, active) {
			break
		}
	}

	if e.stopDelay > 0 {
		time.Sleep(e.stopDelay)
	}
	if err, ok := e.stopErrByID[id]; ok && err != nil {
		return err
	}
	if e.stopRemoves {
		e.mu.Lock()
		delete(e.running, id)
		e.mu.Unlock()
	}
	return nil
}

func (e *drainTestEngine) KillContainer(id string) error {
	if err, ok := e.killErrByID[id]; ok && err != nil {
		return err
	}
	e.mu.Lock()
	delete(e.running, id)
	e.mu.Unlock()
	return nil
}

func (e *drainTestEngine) maxConcurrentStops() int64 {
	return atomic.LoadInt64(&e.maxConcurrent)
}

func TestAdaptiveShutdownDrainTimeout_LockedBaseline(t *testing.T) {
	if got := adaptiveShutdownDrainTimeout(0); got != 30*time.Second {
		t.Fatalf("expected 30s for zero containers, got %s", got)
	}
	if got := adaptiveShutdownDrainTimeout(3); got != 60*time.Second {
		t.Fatalf("expected 60s for 3 containers, got %s", got)
	}
	if got := adaptiveShutdownDrainTimeout(50); got != 180*time.Second {
		t.Fatalf("expected capped 180s timeout, got %s", got)
	}
}

func TestDrainRuntimeForShutdown_SucceedsWithMultipleContainers(t *testing.T) {
	events := captureEvents(t)
	pm := &ProcessManager{
		engine:     newDrainTestEngine("c1", "c2", "c3"),
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	if err := pm.DrainRuntimeForShutdown(0); err != nil {
		t.Fatalf("expected drain success, got err: %v", err)
	}

	var completed *runtimeops.RuntimeEvent
	for i := range *events {
		ev := &(*events)[i]
		if ev.Event == runtimeops.EventShutdownDrain &&
			ev.Message == "shutdown runtime drain complete: no running workload containers" {
			completed = ev
		}
	}
	if completed == nil {
		t.Fatal("expected shutdown.drain completion event")
	}
	if completed.Fields["targetCount"] != 3 {
		t.Fatalf("targetCount=%v", completed.Fields["targetCount"])
	}
	if completed.Fields["stoppedCount"] != 3 {
		t.Fatalf("stoppedCount=%v", completed.Fields["stoppedCount"])
	}
	if completed.Fields["remainingCount"] != 0 {
		t.Fatalf("remainingCount=%v", completed.Fields["remainingCount"])
	}
}

func TestDrainRuntimeForShutdown_TimeoutHasSortedRemainingIDs(t *testing.T) {
	events := captureEvents(t)
	eng := newDrainTestEngine("c3", "c1", "c2")
	eng.stopRemoves = false
	pm := &ProcessManager{
		engine:     eng,
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	err := pm.DrainRuntimeForShutdown(10 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	wantSorted := "c1,c2,c3"
	if !strings.Contains(err.Error(), wantSorted) {
		t.Fatalf("expected sorted remaining IDs %q in error, got %q", wantSorted, err.Error())
	}

	var timedOut *runtimeops.RuntimeEvent
	for i := range *events {
		ev := &(*events)[i]
		if ev.Event == runtimeops.EventShutdownDrain &&
			ev.ReasonCode == runtimeops.ReasonShutdownDrainTimeout {
			timedOut = ev
		}
	}
	if timedOut == nil {
		t.Fatal("expected shutdown.drain timeout event")
	}
	if timedOut.Fields["remainingContainerIds"] != wantSorted {
		t.Fatalf("remainingContainerIds=%v", timedOut.Fields["remainingContainerIds"])
	}
	if timedOut.Fields["targetCount"] != 3 {
		t.Fatalf("targetCount=%v", timedOut.Fields["targetCount"])
	}
	if timedOut.Fields["remainingCount"] != 3 {
		t.Fatalf("remainingCount=%v", timedOut.Fields["remainingCount"])
	}
}

func TestDrainRuntimeForShutdown_UsesBoundedConcurrency(t *testing.T) {
	ids := make([]string, 0, 24)
	for i := 0; i < 24; i++ {
		ids = append(ids, fmt.Sprintf("c%02d", i))
	}
	eng := newDrainTestEngine(ids...)
	eng.stopDelay = 50 * time.Millisecond
	pm := &ProcessManager{
		engine:     eng,
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	if err := pm.DrainRuntimeForShutdown(0); err != nil {
		t.Fatalf("expected drain success, got err: %v", err)
	}
	maxConcurrent := eng.maxConcurrentStops()
	if maxConcurrent <= 1 {
		t.Fatalf("expected concurrent stop workers, maxConcurrent=%d", maxConcurrent)
	}
	if maxConcurrent > shutdownDrainMaxWorkers {
		t.Fatalf("expected bounded concurrency <= %d, got %d", shutdownDrainMaxWorkers, maxConcurrent)
	}
}

func TestDrainRuntimeForShutdown_ZeroContainersFastPath(t *testing.T) {
	events := captureEvents(t)
	pm := &ProcessManager{
		engine:     newDrainTestEngine(),
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	start := time.Now()
	if err := pm.DrainRuntimeForShutdown(0); err != nil {
		t.Fatalf("expected zero-container drain success, got err: %v", err)
	}
	if took := time.Since(start); took > 200*time.Millisecond {
		t.Fatalf("expected fast path under 200ms, got %s", took)
	}

	var completed *runtimeops.RuntimeEvent
	for i := range *events {
		ev := &(*events)[i]
		if ev.Event == runtimeops.EventShutdownDrain &&
			ev.Message == "shutdown runtime drain complete: no running workload containers" {
			completed = ev
		}
	}
	if completed == nil {
		t.Fatal("expected completion event")
	}
	if completed.Fields["targetCount"] != 0 || completed.Fields["remainingCount"] != 0 {
		t.Fatalf("unexpected zero-container completion fields: %+v", completed.Fields)
	}
}
