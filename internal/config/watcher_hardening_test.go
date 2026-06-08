package config

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestWatcherEnqueueReloadCoalescesBurst(t *testing.T) {
	var callbackCount int32
	w := &Watcher{
		callbacks: []func(){
			func() {
				atomic.AddInt32(&callbackCount, 1)
			},
		},
		triggerCh: make(chan struct{}, 1),
	}

	// Burst of events before callback loop starts should coalesce into one trigger.
	for i := 0; i < 25; i++ {
		w.enqueueReload()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.callbackLoop(ctx)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&callbackCount) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := atomic.LoadInt32(&callbackCount); got != 1 {
		t.Fatalf("expected exactly one coalesced callback execution, got %d", got)
	}
}

func TestWatcherShouldHandleEventAtomicRenamePatterns(t *testing.T) {
	w := &Watcher{
		target: "config.yaml",
	}
	basePath := filepath.Join("/tmp", "config.yaml")

	tests := []struct {
		name    string
		event   fsnotify.Event
		wantHit bool
	}{
		{"write target", fsnotify.Event{Name: basePath, Op: fsnotify.Write}, true},
		{"chmod target", fsnotify.Event{Name: basePath, Op: fsnotify.Chmod}, true},
		{"create target", fsnotify.Event{Name: basePath, Op: fsnotify.Create}, true},
		{"rename target", fsnotify.Event{Name: basePath, Op: fsnotify.Rename}, true},
		{"remove target", fsnotify.Event{Name: basePath, Op: fsnotify.Remove}, true},
		{"write non-target", fsnotify.Event{Name: filepath.Join("/tmp", "other.yaml"), Op: fsnotify.Write}, false},
		{"rename non-target", fsnotify.Event{Name: filepath.Join("/tmp", "other.yaml"), Op: fsnotify.Rename}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := w.shouldHandleEvent(tc.event)
			if got != tc.wantHit {
				t.Fatalf("shouldHandleEvent(%v) = %v, want %v", tc.event, got, tc.wantHit)
			}
		})
	}
}
