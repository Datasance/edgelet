package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Ensure RotatingWriter implements io.WriteCloser
var _ io.WriteCloser = (*RotatingWriter)(nil)

// RotatingWriter implements io.WriteCloser with file rotation
// It maintains edgelet.0.log as the active file and rotates to .1.log, .2.log, etc.
type RotatingWriter struct {
	mu          sync.Mutex
	dir         string
	filename    string
	maxSize     int64
	maxBackups  int
	currentFile *os.File
	currentSize int64
}

// NewRotatingWriter creates a new RotatingWriter
// rotateOnExisting: if true, rotate the log file if it already exists and has content (agent restart)
//
//	if false, append to existing file without rotation (config reload)
func NewRotatingWriter(dir, filename string, maxSize int64, maxBackups int, rotateOnExisting bool) (*RotatingWriter, error) {
	w := &RotatingWriter{
		dir:        dir,
		filename:   filename,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}

	if err := w.openCurrentFile(); err != nil {
		return nil, err
	}

	// Only rotate if rotateOnExisting is true AND the file has content
	// This allows us to distinguish between agent restart (should rotate) and config reload (should not rotate)
	if rotateOnExisting && w.currentSize > 0 {
		if err := w.rotate(); err != nil {
			return nil, err
		}
	}

	return w, nil
}

func (w *RotatingWriter) openCurrentFile() error {
	path := filepath.Join(w.dir, fmt.Sprintf("%s.0.log", w.filename))

	// Open file in append mode, create if not exists
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640) // #nosec G302,G304 -- log files are intentionally group-readable; path is filepath.Join(known dir, filename)
	if err != nil {
		return err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close() // cannot use logger here; best-effort close before returning error
		return err
	}

	w.currentFile = file
	w.currentSize = info.Size()
	return nil
}

// Write writes p to the file
func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	writeLen := int64(len(p))
	if w.currentSize+writeLen > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = w.currentFile.Write(p)
	w.currentSize += int64(n)
	return n, err
}

// SetLimits updates rotation caps without rotating or reopening the active file.
func (w *RotatingWriter) SetLimits(maxSize int64, maxBackups int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.maxSize = maxSize
	if maxBackups < 1 {
		maxBackups = 1
	}
	w.maxBackups = maxBackups
}

// Limits returns the current rotation caps.
func (w *RotatingWriter) Limits() (maxSize int64, maxBackups int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maxSize, w.maxBackups
}

// Close closes the file
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentFile != nil {
		return w.currentFile.Close()
	}
	return nil
}

// rotate performs the log rotation
func (w *RotatingWriter) rotate() error {
	if w.currentFile != nil {
		_ = w.currentFile.Close() // cannot use logger here; best-effort close before rotation
	}

	// Rotate existing files: .N -> .N+1
	// Start from maxBackups-1 to avoid overwriting
	for i := w.maxBackups - 1; i >= 0; i-- {
		oldPath := filepath.Join(w.dir, fmt.Sprintf("%s.%d.log", w.filename, i))
		newPath := filepath.Join(w.dir, fmt.Sprintf("%s.%d.log", w.filename, i+1))

		if _, err := os.Stat(oldPath); err == nil {
			// If target exists (from previous rotation or crash), remove it
			if _, err := os.Stat(newPath); err == nil {
				_ = os.Remove(newPath) // cannot use logger here; best-effort remove
			}
			_ = os.Rename(oldPath, newPath) // cannot use logger here; best-effort rename
		}
	}

	// Create new active file (.0.log)
	return w.openCurrentFile()
}
