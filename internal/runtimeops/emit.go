package runtimeops

import (
	"context"
	"strings"
	"sync"

	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const maxErrorLen = 512

// MetricsHook is invoked after each emit for future Prometheus integration (PR-5a deferred).
type MetricsHook interface {
	OnRuntimeEvent(e RuntimeEvent, fields map[string]any)
}

var (
	testSink   func(RuntimeEvent)
	testSinkMu sync.RWMutex

	metricsHook   MetricsHook
	metricsHookMu sync.RWMutex
)

// SetTestSink installs a test-only sink. Pass nil to clear.
func SetTestSink(sink func(RuntimeEvent)) {
	testSinkMu.Lock()
	defer testSinkMu.Unlock()
	testSink = sink
}

// SetMetricsHook installs an optional metrics hook (no-op when nil).
func SetMetricsHook(h MetricsHook) {
	metricsHookMu.Lock()
	defer metricsHookMu.Unlock()
	metricsHook = h
}

// Emit logs a structured runtime event, merging context correlation fields with e.
func Emit(ctx context.Context, e RuntimeEvent) {
	merged := mergeEvent(ctx, e)
	fields := merged.toFieldsMap()

	testSinkMu.RLock()
	sink := testSink
	testSinkMu.RUnlock()
	if sink != nil {
		sink(merged)
	}

	metricsHookMu.RLock()
	mh := metricsHook
	metricsHookMu.RUnlock()
	if mh != nil {
		mh.OnRuntimeEvent(merged, fields)
	}

	level := strings.ToLower(strings.TrimSpace(merged.Level))
	if level == "" {
		level = LevelInfo
	}
	module := strings.TrimSpace(merged.Module)
	if module == "" {
		module = "RuntimeOps"
	}
	msg := strings.TrimSpace(merged.Message)
	if msg == "" {
		msg = merged.Event
	}

	if level == LevelDebug && !debugLogEnabled() {
		return
	}

	var logErr error
	if merged.Error != "" {
		logErr = &truncatedError{msg: merged.Error}
	}
	logging.GetInstance().LogWithFields(level, module, msg, fields, logErr)
}

// mergeEvent combines context operation meta with the event; event fields take precedence.
func mergeEvent(ctx context.Context, e RuntimeEvent) RuntimeEvent {
	meta := OperationFromContext(ctx)
	out := e

	if out.OperationID == "" {
		out.OperationID = meta.OperationID
	}
	if out.Engine == "" {
		out.Engine = meta.Engine
	}
	if out.MsUUID == "" {
		out.MsUUID = meta.MsUUID
	}
	if out.Error != "" {
		out.Error = truncateString(out.Error, maxErrorLen)
	}
	return out
}

// toFieldsMap builds logrus fields (camelCase keys, empty values omitted).
func (e RuntimeEvent) toFieldsMap() map[string]any {
	m := make(map[string]any)
	setStr := func(key, val string) {
		if strings.TrimSpace(val) != "" {
			m[key] = val
		}
	}
	setStr("event", e.Event)
	setStr("operationId", e.OperationID)
	setStr("engine", e.Engine)
	setStr("msUUID", e.MsUUID)
	setStr("containerId", e.ContainerID)
	setStr("sandboxId", e.SandboxID)
	setStr("image", e.Image)
	setStr("phase", e.Phase)
	setStr("result", e.Result)
	setStr("reasonCode", e.ReasonCode)
	setStr("source", e.Source)
	if e.DurationMs > 0 {
		m["durationMs"] = e.DurationMs
	}
	if strings.TrimSpace(e.Error) != "" {
		m["error"] = e.Error
	}
	for k, v := range e.Fields {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		m[k] = v
	}
	return m
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

type truncatedError struct {
	msg string
}

func (e *truncatedError) Error() string {
	return e.msg
}
