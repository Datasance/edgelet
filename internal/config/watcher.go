package config

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const watcherModuleName = "Config Watcher"

// Watcher watches for configuration file changes
type Watcher struct {
	watcher   *fsnotify.Watcher
	path      string
	watchDir  string
	target    string
	mu        sync.RWMutex
	callbacks []func()
	triggerCh chan struct{}
}

var (
	watcherInstance *Watcher
	watcherOnce     sync.Once
)

// GetWatcher returns the singleton watcher instance
func GetWatcher() *Watcher {
	watcherOnce.Do(func() {
		watcherInstance = &Watcher{
			callbacks: make([]func(), 0),
		}
	})
	return watcherInstance
}

// Watch starts watching the configuration file for changes
func (w *Watcher) Watch(ctx context.Context, configPath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.path = configPath
	w.watchDir = filepath.Dir(configPath)
	w.target = filepath.Base(configPath)
	w.triggerCh = make(chan struct{}, 1)

	var err error
	w.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := w.watcher.Add(w.watchDir); err != nil {
		if cerr := w.watcher.Close(); cerr != nil {
			logging.LogWarn(watcherModuleName, fmt.Sprintf("Failed to close watcher: %v", cerr))
		}
		return err
	}

	// Start watching in a goroutine
	go w.watchLoop(ctx)
	go w.callbackLoop(ctx)

	return nil
}

// watchLoop runs the watch loop
func (w *Watcher) watchLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(watcherModuleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()
	for {
		select {
		case <-ctx.Done():
			if err := w.Close(); err != nil {
				logging.LogWarn(watcherModuleName, fmt.Sprintf("Failed to close watcher on context done: %v", err))
			}
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if w.shouldHandleEvent(event) {
				w.enqueueReload()
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			// Log error (we'll use logging system when it's ready)
			_ = err
		}
	}
}

func (w *Watcher) shouldHandleEvent(event fsnotify.Event) bool {
	// Watcher is attached to directory; only trigger for the target config file.
	if filepath.Base(filepath.Clean(event.Name)) != w.target {
		return false
	}
	return event.Op&fsnotify.Write == fsnotify.Write ||
		event.Op&fsnotify.Chmod == fsnotify.Chmod ||
		event.Op&fsnotify.Create == fsnotify.Create ||
		event.Op&fsnotify.Rename == fsnotify.Rename ||
		event.Op&fsnotify.Remove == fsnotify.Remove
}

func (w *Watcher) callbackLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-w.triggerCh:
			if !ok {
				return
			}
			w.runCallbacks()
		}
	}
}

func (w *Watcher) enqueueReload() {
	// Coalesce rapid write/chmod bursts into a single in-flight callback cycle.
	select {
	case w.triggerCh <- struct{}{}:
	default:
	}
}

func (w *Watcher) runCallbacks() {
	w.mu.RLock()
	callbacks := make([]func(), len(w.callbacks))
	copy(callbacks, w.callbacks)
	w.mu.RUnlock()

	for _, callback := range callbacks {
		func(cb func()) {
			defer func() {
				if r := recover(); r != nil {
					logging.LogError(watcherModuleName, "Panic recovered in config callback", fmt.Errorf("%v", r))
				}
			}()
			cb()
		}(callback)
	}
}

// OnChange registers a callback to be called when config changes
func (w *Watcher) OnChange(callback func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, callback)
}

// Close closes the watcher
func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.watcher != nil {
		if err := w.watcher.Close(); err != nil {
			return err
		}
	}
	if w.triggerCh != nil {
		close(w.triggerCh)
		w.triggerCh = nil
	}
	return nil
}
