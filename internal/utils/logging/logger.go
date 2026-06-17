package logging

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// Logger is the main logger interface
type Logger interface {
	Debug(moduleName, msg string)
	Info(moduleName, msg string)
	Warn(moduleName, msg string)
	Error(moduleName, msg string, err error)
	SetLevel(level string)
	GetLevel() logrus.Level
}

const (
	// BasenameControlPlane is the log file series for edgelet.service.
	BasenameControlPlane = "edgelet"
	// BasenameDataPlane is the log file series for edgelet-containerd.service.
	BasenameDataPlane = "edgelet-containerd"
)

// LogrusLogger wraps logrus.Logger with module support
type LogrusLogger struct {
	logger        *logrus.Logger
	logWriter     io.WriteCloser
	basename      string
	isInitialized bool
	mu            sync.RWMutex
}

func resolveBasename(basename ...string) string {
	if len(basename) > 0 && basename[0] != "" {
		return basename[0]
	}
	return BasenameControlPlane
}

var (
	instance *LogrusLogger
	once     sync.Once
)

// GetInstance returns the singleton logger instance
func GetInstance() *LogrusLogger {
	once.Do(func() {
		instance = &LogrusLogger{
			logger: logrus.New(),
		}
	})
	return instance
}

// SetupLogger sets up the logger with file rotation and console output.
// Optional basename selects the log file series (default BasenameControlPlane).
func SetupLogger(logDir string, maxFileSizeMB int, logFileCount int, logLevel string, basename ...string) error {
	logger := GetInstance()
	logger.mu.Lock()
	defer logger.mu.Unlock()

	logBasename := resolveBasename(basename...)

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return err
	}

	maxFileSize := computeMaxFileSize(maxFileSizeMB, logFileCount)

	// Setup file rotation
	// Only rotate on existing file if this is the first initialization (edgelet restart)
	rotateOnExisting := !logger.isInitialized
	logFile, err := NewRotatingWriter(logDir, logBasename, maxFileSize, logFileCount, rotateOnExisting)
	if err != nil {
		return err
	}

	// Set formatter: JSON by default for enterprise observability (structured, parseable)
	logger.logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyMsg:  "message",
			logrus.FieldKeyTime: "timestamp",
		},
	})

	// Set output to both file and console
	// Do this BEFORE closing the old writer to prevent any race conditions where logrus might try to write to a closed file
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	logger.logger.SetOutput(multiWriter)

	// Close previous writer if it exists to prevent resource leaks
	if logger.logWriter != nil {
		_ = logger.logWriter.Close() // cannot use logger here (circular); best-effort close
	}
	logger.logWriter = logFile
	logger.basename = logBasename

	// Set log level (logrus expects lowercase, but config stores uppercase)
	// Convert to lowercase before parsing
	level, err := logrus.ParseLevel(strings.ToLower(logLevel))
	if err != nil {
		// Default to InfoLevel if parsing fails
		level = logrus.InfoLevel
	}
	logger.logger.SetLevel(level)

	// Mark as initialized after first setup
	logger.isInitialized = true

	return nil
}

// Debug logs a debug message
func (l *LogrusLogger) Debug(moduleName, msg string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.logger.WithField("module", moduleName).Debug(msg)
}

// Info logs an info message
func (l *LogrusLogger) Info(moduleName, msg string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.logger.WithField("module", moduleName).Info(msg)
}

// Warn logs a warning message
func (l *LogrusLogger) Warn(moduleName, msg string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.logger.WithField("module", moduleName).Warn(msg)
}

// Error logs an error message
func (l *LogrusLogger) Error(moduleName, msg string, err error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if err != nil {
		l.logger.WithField("module", moduleName).WithError(err).Error(msg)
	} else {
		l.logger.WithField("module", moduleName).Error(msg)
	}
}

// LogWithFields logs a message with structured fields as top-level JSON keys (enterprise SIEM).
// level is one of: debug, info, warn, error.
func (l *LogrusLogger) LogWithFields(level, moduleName, msg string, fields map[string]any, err error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entry := l.logger.WithField("module", moduleName)
	for k, v := range fields {
		entry = entry.WithField(k, v)
	}
	if err != nil {
		entry = entry.WithError(err)
	}

	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		entry.Debug(msg)
	case "warn", "warning":
		entry.Warn(msg)
	case "error":
		entry.Error(msg)
	default:
		entry.Info(msg)
	}
}

// SetLevel sets the log level
// logrus expects lowercase level names (e.g., "debug", "info"), but config stores uppercase
func (l *LogrusLogger) SetLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Convert to lowercase before parsing
	if parsedLevel, err := logrus.ParseLevel(strings.ToLower(level)); err == nil {
		l.logger.SetLevel(parsedLevel)
	}
}

// GetLevel returns the current log level
func (l *LogrusLogger) GetLevel() logrus.Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.logger.GetLevel()
}

// UpdateLoggerConfig updates logger configuration without recreating the writer.
// This prevents log rotation on config reloads. Optional basename is ignored.
func UpdateLoggerConfig(_ string, maxFileSizeMB int, logFileCount int, logLevel string, _ ...string) error {
	logger := GetInstance()
	logger.mu.Lock()
	defer logger.mu.Unlock()

	logger.logger.WithField("module", "LoggingService").Debug("Updating logger configuration without rotation")

	if rw, ok := logger.logWriter.(*RotatingWriter); ok {
		rw.SetLimits(computeMaxFileSize(maxFileSizeMB, logFileCount), logFileCount)
	}

	level, err := logrus.ParseLevel(strings.ToLower(logLevel))
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.logger.SetLevel(level)

	return nil
}

// GetRotatingWriterLimits returns rotation caps for the daemon log writer, if present.
func GetRotatingWriterLimits() (maxSize int64, maxBackups int, ok bool) {
	logger := GetInstance()
	logger.mu.RLock()
	defer logger.mu.RUnlock()
	rw, isRotating := logger.logWriter.(*RotatingWriter)
	if !isRotating {
		return 0, 0, false
	}
	size, backups := rw.Limits()
	return size, backups, true
}

// InstanceConfigUpdated updates the logger with updated configuration
// which reuses existing FileHandler
func InstanceConfigUpdated(logDir string, maxFileSizeMB int, logFileCount int, logLevel string, basename ...string) error {
	// Use UpdateLoggerConfig instead of SetupLogger to prevent rotation on config reload
	return UpdateLoggerConfig(logDir, maxFileSizeMB, logFileCount, logLevel, basename...)
}

// LogDebug is a convenience function for debug logging
func LogDebug(moduleName, msg string) {
	GetInstance().Debug(moduleName, msg)
}

// LogInfo is a convenience function for info logging
func LogInfo(moduleName, msg string) {
	GetInstance().Info(moduleName, msg)
}

// LogWarn is a convenience function for warning logging
func LogWarn(moduleName, msg string) {
	GetInstance().Warn(moduleName, msg)
}

// LogError is a convenience function for error logging
func LogError(moduleName, msg string, err error) {
	GetInstance().Error(moduleName, msg, err)
}
