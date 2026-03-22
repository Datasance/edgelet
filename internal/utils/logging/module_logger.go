package logging

import (
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

// MicroserviceLogger manages loggers for individual microservices
type MicroserviceLogger struct {
	loggers    map[string]*logrus.Logger
	logWriters map[string]io.WriteCloser
	mu         sync.RWMutex
	logDir     string
}

var (
	microserviceLoggerInstance *MicroserviceLogger
	microserviceLoggerOnce     sync.Once
)

// GetMicroserviceLoggerInstance returns the singleton microservice logger instance
func GetMicroserviceLoggerInstance() *MicroserviceLogger {
	microserviceLoggerOnce.Do(func() {
		microserviceLoggerInstance = &MicroserviceLogger{
			loggers:    make(map[string]*logrus.Logger),
			logWriters: make(map[string]io.WriteCloser),
		}
	})
	return microserviceLoggerInstance
}

// SetupMicroserviceLogger sets up a logger for a specific microservice
func SetupMicroserviceLogger(microserviceUUID string, logDir string, maxFileSizeMB int, logFileCount int) error {
	ml := GetMicroserviceLoggerInstance()
	ml.mu.Lock()
	defer ml.mu.Unlock()

	ml.logDir = logDir

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return err
	}

	// Calculate max file size per file
	maxFileSize := (maxFileSizeMB * 1024 * 1024) / logFileCount
	if maxFileSize < 1024*1024 {
		maxFileSize = 1024 * 1024 // Minimum 1MB
	}

	// Setup file rotation (matching Java behavior: {uuid}.0.log as active)
	// Rotate on existing files for initial setup (similar to main logger)
	logFile, err := NewRotatingWriter(logDir, microserviceUUID, int64(maxFileSize), logFileCount, true)
	if err != nil {
		return err
	}

	// Create new logger
	logger := logrus.New()
	logger.SetFormatter(&MicroserviceLogFormatter{})
	logger.SetOutput(io.MultiWriter(os.Stdout, logFile))
	logger.SetLevel(logrus.InfoLevel)

	// Close existing writer if any
	if oldWriter, exists := ml.logWriters[microserviceUUID]; exists {
		_ = oldWriter.Close() // cannot use logger here (circular); best-effort close
	}

	ml.loggers[microserviceUUID] = logger
	ml.logWriters[microserviceUUID] = logFile

	return nil
}

// LogInfo logs an info message for a microservice
func (ml *MicroserviceLogger) LogInfo(microserviceUUID, msg string) bool {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	logger, exists := ml.loggers[microserviceUUID]
	if !exists {
		LogWarn("MicroserviceLogger", "Logger not initialized for microservice: "+microserviceUUID)
		return false
	}

	logger.Info(msg)
	return true
}

// LogWarn logs a warning message for a microservice
func (ml *MicroserviceLogger) LogWarn(microserviceUUID, msg string) bool {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	logger, exists := ml.loggers[microserviceUUID]
	if !exists {
		LogWarn("MicroserviceLogger", "Logger not initialized for microservice: "+microserviceUUID)
		return false
	}

	logger.Warn(msg)
	return true
}

// LogError logs an error message for a microservice
func (ml *MicroserviceLogger) LogError(microserviceUUID, msg string, err error) bool {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	logger, exists := ml.loggers[microserviceUUID]
	if !exists {
		LogWarn("MicroserviceLogger", "Logger not initialized for microservice: "+microserviceUUID)
		return false
	}

	if err != nil {
		logger.WithError(err).Error(msg)
	} else {
		logger.Error(msg)
	}
	return true
}

// Note: InstanceConfigUpdated is now in logger.go to re-initialize the main logger
// This file only handles microservice-specific loggers
