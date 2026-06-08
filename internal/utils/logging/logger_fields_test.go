package logging

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestLogWithFields_TopLevelKeysInJSON(t *testing.T) {
	l := &LogrusLogger{logger: logrus.New()}
	l.logger.SetFormatter(&logrus.JSONFormatter{})
	var buf bytes.Buffer
	l.logger.SetOutput(&buf)

	l.LogWithFields("info", "TestModule", "hello", map[string]any{
		"event":       "container.started",
		"operationId": "op-1",
		"msUUID":      "ms-1",
	}, nil)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("unmarshal: %v raw=%s", err, buf.String())
	}
	if line["event"] != "container.started" {
		t.Fatalf("event=%v", line["event"])
	}
	if line["operationId"] != "op-1" {
		t.Fatalf("operationId=%v", line["operationId"])
	}
	if line["module"] != "TestModule" {
		t.Fatalf("module=%v", line["module"])
	}
}
