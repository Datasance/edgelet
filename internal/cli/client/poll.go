package client

import (
	"context"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/ui"
)

const defaultPollInterval = 500 * time.Millisecond

// PollConfig controls async operation polling.
type PollConfig struct {
	Interval    time.Duration
	Timeout     time.Duration
	PercentStep int
}

// PollProgress configures stderr progress rendering during polling.
type PollProgress struct {
	UI             *ui.UI
	Spinner        *ui.Spinner
	StageFormatter func(stage string) string
	PercentLabel   string
}

// PollAsyncOperation polls fetch until the operation reaches a terminal status.
// When StageFormatter is set, stage transitions are rendered on stderr.
// When PercentLabel is set, percent progress is rendered instead.
func PollAsyncOperation(ctx context.Context, cfg PollConfig, fetch func() (map[string]interface{}, error), progress PollProgress) (map[string]interface{}, []string, error) {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultPollInterval
	}
	if cfg.PercentStep <= 0 {
		cfg.PercentStep = 5
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var (
		stages       []string
		lastStage    string
		lastLine     string
		lastProgress = -1
		deadline     time.Time
	)
	if cfg.Timeout > 0 {
		deadline = time.Now().Add(cfg.Timeout)
	}

	for {
		if err := ctx.Err(); err != nil {
			clearProgress(progress)
			return nil, stages, err
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			clearProgress(progress)
			return nil, stages, ErrPollTimeout
		}

		data, err := fetch()
		if err != nil {
			clearProgress(progress)
			return nil, stages, err
		}

		status := normalizeStatus(output.MapValueAsString(data, "status"))
		if progress.StageFormatter != nil {
			stage := normalizeStage(output.MapValueAsString(data, "stage"))
			if stage != "" && stage != lastStage {
				stages = appendStage(stages, stage)
				lastStage = stage
				line := progress.StageFormatter(stage)
				if progress.UI != nil && line != "" && line != lastLine {
					if progress.Spinner != nil && progress.UI.Interactive() {
						progress.Spinner.SetSuffix(line)
					} else {
						progress.UI.WriteStageLine(line)
					}
					lastLine = line
				}
			}
		}
		if progress.PercentLabel != "" && progress.UI != nil {
			if percent, ok := parsePercent(data); ok {
				if lastProgress < 0 || percent-lastProgress >= cfg.PercentStep || percent == 100 {
					progress.UI.WritePercent(progress.PercentLabel, percent, false)
					lastProgress = percent
				}
			}
		}

		switch status {
		case "succeeded":
			if progress.PercentLabel != "" && progress.UI != nil {
				progress.UI.WritePercent(progress.PercentLabel, 100, true)
			} else {
				clearProgress(progress)
			}
			return data, stages, nil
		case "failed":
			clearProgress(progress)
			return data, stages, nil
		case "queued", "running", "":
			time.Sleep(cfg.Interval)
			continue
		default:
			time.Sleep(cfg.Interval)
		}
	}
}

// ErrPollTimeout indicates polling exceeded the configured timeout.
var ErrPollTimeout = &PollError{Message: "operation polling timed out"}

// PollError is a polling failure.
type PollError struct {
	Message string
}

func (e *PollError) Error() string {
	if e == nil || e.Message == "" {
		return "poll error"
	}
	return e.Message
}

func clearProgress(progress PollProgress) {
	if progress.Spinner != nil {
		progress.Spinner.Stop()
		return
	}
	if progress.UI != nil {
		progress.UI.ClearProgressLine()
	}
}

func appendStage(stages []string, stage string) []string {
	if len(stages) > 0 && stages[len(stages)-1] == stage {
		return stages
	}
	return append(stages, stage)
}

func normalizeStatus(raw string) string {
	status := strings.TrimSpace(strings.ToLower(raw))
	if status == "" || status == "<unknown>" {
		return "running"
	}
	return status
}

func normalizeStage(raw string) string {
	stage := strings.TrimSpace(strings.ToLower(raw))
	if stage == "" || stage == "<unknown>" {
		return ""
	}
	return stage
}

func parsePercent(data map[string]interface{}) (int, bool) {
	if data == nil {
		return 0, false
	}
	raw, ok := data["progress"]
	if !ok {
		return 0, false
	}
	switch typed := raw.(type) {
	case float64:
		return clampPercent(int(typed)), true
	case int:
		return clampPercent(typed), true
	case int64:
		return clampPercent(int(typed)), true
	default:
		return 0, false
	}
}

func clampPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

// OperationIDFromStart extracts an async operation id from a start response.
func OperationIDFromStart(data map[string]interface{}) string {
	return strings.TrimSpace(output.MapValueAsString(data, "operationId"))
}
