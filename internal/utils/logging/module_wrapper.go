package logging

import "fmt"

// ModuleLogger is a simple wrapper for module-specific logging
type ModuleLogger struct {
	moduleName string
}

// NewModuleLogger creates a new ModuleLogger for a specific module
func NewModuleLogger(moduleName string) *ModuleLogger {
	return &ModuleLogger{
		moduleName: moduleName,
	}
}

// Debug logs a debug message
func (ml *ModuleLogger) Debug(msg string) {
	LogDebug(ml.moduleName, msg)
}

// Debugf logs a formatted debug message
func (ml *ModuleLogger) Debugf(format string, args ...any) {
	LogDebug(ml.moduleName, fmt.Sprintf(format, args...))
}

// Info logs an info message
func (ml *ModuleLogger) Info(msg string) {
	LogInfo(ml.moduleName, msg)
}

// Infof logs a formatted info message
func (ml *ModuleLogger) Infof(format string, args ...any) {
	LogInfo(ml.moduleName, fmt.Sprintf(format, args...))
}

// Warn logs a warning message
func (ml *ModuleLogger) Warn(msg string) {
	LogWarn(ml.moduleName, msg)
}

// Warnf logs a formatted warning message
func (ml *ModuleLogger) Warnf(format string, args ...any) {
	LogWarn(ml.moduleName, fmt.Sprintf(format, args...))
}

// Error logs an error message
func (ml *ModuleLogger) Error(msg string) {
	LogError(ml.moduleName, msg, nil)
}

// Errorf logs a formatted error message
func (ml *ModuleLogger) Errorf(format string, args ...any) {
	LogError(ml.moduleName, fmt.Sprintf(format, args...), nil)
}
