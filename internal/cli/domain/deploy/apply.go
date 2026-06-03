package deploy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/cli/client"
	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/datasance/edgelet/internal/cli/ui"
)

const runtimeClassApplyPollTimeout = 90 * time.Second

var (
	runtimeClassApplyPollInterval = time.Second
	startMultipartApply           = func(api run.EdgeletAPIClient, target Target, manifestPath string, fields map[string]string) (map[string]interface{}, error) {
		return api.RequestMultipartFile("POST", target.applyPath(), "manifest", manifestPath, fields)
	}
	fetchApplyStatus = func(api run.EdgeletAPIClient, target Target, operationID string) (map[string]interface{}, error) {
		return api.Request("GET", target.applyStatusPath(operationID), nil)
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
func Execute(ctx context.Context, api run.EdgeletAPIClient, uiProgress *ui.UI, req Request) (*Result, error) {
	if api == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	if strings.TrimSpace(req.ManifestPath) == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: edgelet deploy -f <manifest.yaml>", nil)
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

	switch target {
	case TargetControlPlane:
		result, err := applyAsync(ctx, api, uiProgress, target, req.ManifestPath, fields)
		if err != nil {
			if errors.Is(err, client.ErrPollTimeout) {
				return nil, err
			}
			return nil, err
		}
		if !req.DryRun {
			if err := verifyControlPlaneRunning(api); err != nil {
				return nil, err
			}
		}
		return result, nil
	case TargetMicroservices, TargetRuntimeClasses:
		if req.DryRun {
			data, err := api.RequestMultipartFile("POST", target.validatePath(), "manifest", req.ManifestPath, fields)
			if err != nil {
				return nil, run.MapAPIError(err)
			}
			return &Result{Data: data, Human: FormatValidateHuman(data)}, nil
		}
		fields["async"] = "true"
		return applyAsync(ctx, api, uiProgress, target, req.ManifestPath, fields)
	case TargetRegistries:
		var spin *ui.Spinner
		if uiProgress != nil {
			spin = uiProgress.StartSpinner(applySpinnerMessage(target))
			defer spin.Stop()
		}
		data, err := api.RequestMultipartFile("POST", target.applyPath(), "manifest", req.ManifestPath, fields)
		if err != nil {
			return nil, run.MapAPIError(err)
		}
		return &Result{Data: data, Human: FormatApplyHuman(data)}, nil
	default:
		return nil, run.NewCLIError(run.CodeInternal, "unsupported deploy target", nil)
	}
}

func verifyControlPlaneRunning(api run.EdgeletAPIClient) error {
	data, err := api.Request("GET", "/v1/system/controlplane", nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	state := strings.ToLower(strings.TrimSpace(output.MapValueAsString(data, "runtimeState")))
	if state == "running" {
		return nil
	}
	return run.NewCLIError(
		run.CodeInternal,
		fmt.Sprintf("control plane apply finished but runtimeState=%q (expected running); run edgelet controlplane get", state),
		nil,
	)
}

func controlPlanePollTimeoutError(operationID string) error {
	msg := "control plane deploy polling timed out; work may still be running on the daemon"
	msg += "; check edgelet controlplane get"
	if strings.TrimSpace(operationID) != "" {
		msg += fmt.Sprintf(" or GET /v1/deploy/controlplane:apply/%s", strings.TrimSpace(operationID))
	}
	msg += "; do not run another deploy immediately"
	return run.NewCLIError(run.CodeInternal, msg, client.ErrPollTimeout)
}

func applySpinnerMessage(target Target) string {
	switch target {
	case TargetControlPlane:
		return "Applying control plane manifest..."
	case TargetRegistries:
		return "Applying registry manifest..."
	default:
		return "Applying manifest..."
	}
}

func applyAsync(ctx context.Context, api run.EdgeletAPIClient, uiProgress *ui.UI, target Target, manifestPath string, fields map[string]string) (*Result, error) {
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
	switch target {
	case TargetRuntimeClasses:
		pollCfg.Interval = runtimeClassApplyPollInterval
		pollCfg.Timeout = runtimeClassApplyPollTimeout
	case TargetControlPlane:
		pollCfg.Timeout = client.PollTimeoutFor("controlplane")
	default:
		pollCfg.Timeout = client.PollTimeoutFor("microservices")
	}

	stageFormatter := ui.FormatDeployStageLine
	baseMessage := "Applying microservice manifest..."
	switch target {
	case TargetRuntimeClasses:
		stageFormatter = ui.FormatRuntimeClassStageLine
		baseMessage = "Applying runtimeclass manifest..."
	case TargetControlPlane:
		stageFormatter = ui.FormatControlPlaneStageLine
		baseMessage = "Applying control plane manifest..."
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
		if err == client.ErrPollTimeout && target == TargetControlPlane {
			return nil, controlPlanePollTimeoutError(operationID)
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
