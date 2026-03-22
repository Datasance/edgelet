package config

import (
	"context"
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const watcherModuleName = "Config Watcher"

// Watcher watches for configuration file changes
type Watcher struct {
	watcher   *fsnotify.Watcher
	path      string
	mu        sync.RWMutex
	callbacks []func()
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

	var err error
	w.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := w.watcher.Add(configPath); err != nil {
		if cerr := w.watcher.Close(); cerr != nil {
			logging.LogWarn(watcherModuleName, fmt.Sprintf("Failed to close watcher: %v", cerr))
		}
		return err
	}

	// Start watching in a goroutine
	go w.watchLoop(ctx)

	return nil
}

// watchLoop runs the watch loop
func (w *Watcher) watchLoop(ctx context.Context) {
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
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Chmod == fsnotify.Chmod {
				// Config file was modified, reload it
				// Note: Some editors trigger CHMOD events on write
				// Reload config logic moved to callback execution

				// Notify all callbacks
				w.mu.RLock()
				callbacks := make([]func(), len(w.callbacks))
				copy(callbacks, w.callbacks)
				w.mu.RUnlock()

				for _, callback := range callbacks {
					go callback()
				}
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
		return w.watcher.Close()
	}
	return nil
}
