package deploy

import (
	"context"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/cli/client"
	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/eclipse-iofog/agent/internal/cli/ui"
)

const runtimeClassApplyPollTimeout = 90 * time.Second

var (
	runtimeClassApplyPollInterval = time.Second
	startMultipartApply           = func(api run.V3Client, target Target, manifestPath string, fields map[string]string) (map[string]interface{}, error) {
		return api.RequestV3MultipartFile("POST", target.applyPath(), "manifest", manifestPath, fields)
	}
	fetchApplyStatus = func(api run.V3Client, target Target, operationID string) (map[string]interface{}, error) {
		return api.RequestV3("GET", target.applyStatusPath(operationID), nil)
	}
)

// Request carries deploy -f options.
type Request struct {
	ManifestPath string
	SourceName   string
	DryRun       bool
}

// Result is the deploy command outcome.
type Result struct {
	Data   map[string]interface{}
	Stages []string
	Human  string
}

// Execute runs deploy validate or apply for a manifest file.
func Execute(ctx context.Context, api run.V3Client, uiProgress *ui.UI, req Request) (*Result, error) {
	if api == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	if strings.TrimSpace(req.ManifestPath) == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent deploy -f <manifest.yaml>", nil)
	}

	target, err := DetectTargetFromManifest(req.ManifestPath)
	if err != nil {
		return nil, run.NewCLIError(run.CodeInvalidArgument, err.Error(), err)
	}

	fields := map[string]string{}
	if req.SourceName != "" {
		fields["sourceName"] = req.SourceName
	}
	if req.DryRun {
		fields["dryRun"] = "true"
	}

	if req.DryRun {
		data, err := api.RequestV3MultipartFile("POST", target.validatePath(), "manifest", req.ManifestPath, fields)
		if err != nil {
			return nil, run.MapAPIError(err)
		}
		return &Result{Data: data, Human: FormatValidateHuman(data)}, nil
	}

	switch target {
	case TargetMicroservices, TargetRuntimeClasses:
		fields["async"] = "true"
		return applyAsync(ctx, api, uiProgress, target, req.ManifestPath, fields)
	default:
		var spin *ui.Spinner
		if uiProgress != nil {
			spin = uiProgress.StartSpinner("Applying registry manifest...")
			defer spin.Stop()
		}
		data, err := api.RequestV3MultipartFile("POST", target.applyPath(), "manifest", req.ManifestPath, fields)
		if err != nil {
			return nil, run.MapAPIError(err)
		}
		return &Result{Data: data, Human: FormatApplyHuman(data)}, nil
	}
}

func applyAsync(ctx context.Context, api run.V3Client, uiProgress *ui.UI, target Target, manifestPath string, fields map[string]string) (*Result, error) {
	startResult, err := startMultipartApply(api, target, manifestPath, fields)
	if err != nil {
		return nil, run.MapAPIError(err)
	}

	startStatus := normalizeStatus(output.MapValueAsString(startResult, "status"))
	if startStatus == "succeeded" {
		return finalizeApply(startResult, nil)
	}
	if startStatus == "failed" {
		code, message := ApplyError(startResult)
		return nil, run.NewCLIError(code, message, nil)
	}

	operationID := client.OperationIDFromStart(startResult)
	if operationID == "" || operationID == "<unknown>" {
		return finalizeApply(startResult, nil)
	}

	pollCfg := client.PollConfig{Interval: 500 * time.Millisecond}
	if target == TargetRuntimeClasses {
		pollCfg.Interval = runtimeClassApplyPollInterval
		pollCfg.Timeout = runtimeClassApplyPollTimeout
	}

	stageFormatter := ui.FormatDeployStageLine
	baseMessage := "Applying microservice manifest..."
	if target == TargetRuntimeClasses {
		stageFormatter = ui.FormatRuntimeClassStageLine
		baseMessage = "Applying runtimeclass manifest..."
	}

	progress := client.PollProgress{
		UI:             uiProgress,
		StageFormatter: stageFormatter,
	}
	if uiProgress != nil {
		spin := uiProgress.StartSpinner(baseMessage)
		defer spin.Stop()
		progress.Spinner = spin
	}

	final, stages, err := client.PollAsyncOperation(ctx, pollCfg, func() (map[string]interface{}, error) {
		return fetchApplyStatus(api, target, operationID)
	}, progress)
	if err != nil {
		if err == client.ErrPollTimeout && target == TargetRuntimeClasses {
			human := FormatRuntimeClassInProgress(operationID, "running", lastStageFrom(stages))
			data := map[string]interface{}{
				"operationId": operationID,
				"status":      "running",
			}
			return &Result{Data: WithStages(data, stages), Stages: stages, Human: human}, nil
		}
		return nil, run.MapAPIError(err)
	}

	status := normalizeStatus(output.MapValueAsString(final, "status"))
	if status == "failed" {
		code, message := ApplyError(final)
		return nil, run.NewCLIError(code, message, nil)
	}
	return finalizeApply(final, stages)
}

func lastStageFrom(stages []string) string {
	if len(stages) == 0 {
		return ""
	}
	return stages[len(stages)-1]
}

func finalizeApply(data map[string]interface{}, stages []string) (*Result, error) {
	human := FormatApplyHuman(data)
	if strings.HasPrefix(human, "Error[") {
		code, message := ApplyError(data)
		return nil, run.NewCLIError(code, message, nil)
	}
	return &Result{
		Data:   WithStages(data, stages),
		Stages: stages,
		Human:  human,
	}, nil
}
