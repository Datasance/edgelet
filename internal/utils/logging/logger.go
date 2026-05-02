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

// LogrusLogger wraps logrus.Logger with module support
type LogrusLogger struct {
	logger        *logrus.Logger
	logWriter     io.WriteCloser
	isInitialized bool
	mu            sync.RWMutex
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

// SetupLogger sets up the logger with file rotation and console output
func SetupLogger(logDir string, maxFileSizeMB int, logFileCount int, logLevel string) error {
	logger := GetInstance()
	logger.mu.Lock()
	defer logger.mu.Unlock()

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return err
	}

	// Calculate max file size per file
	maxFileSize := (maxFileSizeMB * 1024 * 1024) / logFileCount
	if maxFileSize < 1024*1024 {
		maxFileSize = 1024 * 1024 // Minimum 1MB
	}
	if maxFileSize > 2*1024*1024*1024 {
		maxFileSize = 2 * 1024 * 1024 * 1024 // Maximum 2GB
	}

	// Setup file rotation (matching Java behavior: iofog-agent.0.log as active)
	// Only rotate on existing file if this is the first initialization (agent restart)
	rotateOnExisting := !logger.isInitialized
	logFile, err := NewRotatingWriter(logDir, "iofog-agent", int64(maxFileSize), logFileCount, rotateOnExisting)
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

	// Set log level (logrus expects lowercase, but config stores uppercase)
	// Convert to lowercase before parsing (matching Java: Level.parse(logLevel))
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

// SetLevel sets the log level
// logrus expects lowercase level names (e.g., "debug", "info"), but config stores uppercase
func (l *LogrusLogger) SetLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Convert to lowercase before parsing (matching Java: Level.parse(logLevel))
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

// UpdateLoggerConfig updates logger configuration without recreating the writer
// This prevents log rotation on config reloads (matching Java: reuses existing FileHandler)
func UpdateLoggerConfig(_ string, _ int, _ int, logLevel string) error {
	logger := GetInstance()
	logger.mu.Lock()
	defer logger.mu.Unlock()

	// Only update log level and directory if changed, but don't recreate the writer
	// This prevents log rotation on config reloads
	logger.logger.WithField("module", "LoggingService").Debug("Updating logger configuration without rotation")

	// Set log level (logrus expects lowercase, but config stores uppercase)
	level, err := logrus.ParseLevel(strings.ToLower(logLevel))
	if err != nil {
		// Default to InfoLevel if parsing fails
		level = logrus.InfoLevel
	}
	logger.logger.SetLevel(level)

	// Note: We don't recreate the RotatingWriter here to avoid rotation
	// The existing writer will continue to write to the same file
	// If directory or file size limits changed, they'll take effect on next rotation

	return nil
}

// InstanceConfigUpdated updates the logger with updated configuration
// Matching Java: LoggingService.instanceConfigUpdated() which reuses existing FileHandler
func InstanceConfigUpdated(logDir string, maxFileSizeMB int, logFileCount int, logLevel string) error {
	// Use UpdateLoggerConfig instead of SetupLogger to prevent rotation on config reload
	return UpdateLoggerConfig(logDir, maxFileSizeMB, logFileCount, logLevel)
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
