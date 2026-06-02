package logging

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// LogFormatter formats log entries
type LogFormatter struct{}

// Format formats a log entry
func (f *LogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format("2006-01-02 15:04:05.000")
	level := entry.Level.String()
	module := entry.Data["module"]
	message := entry.Message

	var output string
	if err, ok := entry.Data["error"].(error); ok && err != nil {
		output = fmt.Sprintf("%s [%s] %s: %s - Error: %s\n",
			timestamp, level, module, message, err.Error())
	} else {
		output = fmt.Sprintf("%s [%s] %s: %s\n",
			timestamp, level, module, message)
	}

	return []byte(output), nil
}

// MicroserviceLogFormatter formats log entries for microservices
type MicroserviceLogFormatter struct{}

// Format formats a microservice log entry
func (f *MicroserviceLogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format(time.RFC3339)
	level := entry.Level.String()
	message := entry.Message

	output := fmt.Sprintf("%s [%s] %s\n", timestamp, level, message)
	return []byte(output), nil
}
