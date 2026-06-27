package handlers

import (
	"strings"
	"time"
)

func snapshotImagePullOperation(op *imagePullOperation) map[string]any {
	response := map[string]any{
		"operationId":   op.OperationID,
		"status":        op.Status,
		"progress":      op.Progress,
		"image":         op.Image,
		"resolvedImage": op.ResolvedImage,
		"platform":      op.Platform,
		"engine":        op.Engine,
		"startedAt":     op.StartedAt.Format(time.RFC3339Nano),
	}
	if op.RegistryID != nil {
		response["registryId"] = *op.RegistryID
	}
	if op.EndedAt != nil {
		response["endedAt"] = op.EndedAt.Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(op.Error) != "" {
		response["error"] = op.Error
	}
	return response
}

func snapshotImageLoadOperation(op *imageLoadOperation) map[string]any {
	response := map[string]any{
		"operationId": op.OperationID,
		"status":      op.Status,
		"path":        op.Path,
		"engine":      op.Engine,
		"startedAt":   op.StartedAt.Format(time.RFC3339Nano),
	}
	if len(op.Loaded) > 0 {
		response["loaded"] = op.Loaded
		response["count"] = op.Count
	}
	if op.EndedAt != nil {
		response["endedAt"] = op.EndedAt.Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(op.Error) != "" {
		response["error"] = op.Error
	}
	return response
}

func snapshotDeployApplyOperation(op *deployApplyOperation) map[string]any {
	response := map[string]any{
		"operationId": op.OperationID,
		"status":      op.Status,
		"startedAt":   op.StartedAt.Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(op.Stage) != "" {
		response["stage"] = op.Stage
	}
	if strings.TrimSpace(op.Kind) != "" {
		response["kind"] = op.Kind
	}
	if strings.TrimSpace(op.Name) != "" {
		response["name"] = op.Name
	}
	if strings.TrimSpace(op.Image) != "" {
		response["image"] = op.Image
	}
	if strings.TrimSpace(op.DeploymentID) != "" {
		response["deploymentId"] = op.DeploymentID
	}
	if op.EndedAt != nil {
		response["endedAt"] = op.EndedAt.Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(op.ErrorMessage) != "" {
		code := strings.TrimSpace(op.ErrorCode)
		if code == "" {
			code = ErrCodeInternal
		}
		response["error"] = map[string]any{
			"code":    code,
			"message": op.ErrorMessage,
		}
	}
	return response
}

func snapshotDeployApplyAccepted(op *deployApplyOperation) map[string]any {
	return map[string]any{
		"operationId": op.OperationID,
		"status":      op.Status,
		"kind":        op.Kind,
		"name":        op.Name,
		"image":       op.Image,
		"startedAt":   op.StartedAt.Format(time.RFC3339Nano),
	}
}

func snapshotControlPlaneApplyOperation(op *controlPlaneApplyOperation) map[string]any {
	response := map[string]any{
		"operationId": op.OperationID,
		"status":      op.Status,
		"startedAt":   op.StartedAt.Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(op.Stage) != "" {
		response["stage"] = op.Stage
	}
	if strings.TrimSpace(op.ControllerUUID) != "" {
		response["controllerUuid"] = op.ControllerUUID
	}
	if op.Generation > 0 {
		response["generation"] = op.Generation
	}
	if strings.TrimSpace(op.Namespace) != "" {
		response["namespace"] = op.Namespace
	}
	if strings.TrimSpace(op.Name) != "" {
		response["name"] = op.Name
	}
	if strings.TrimSpace(op.Image) != "" {
		response["image"] = op.Image
	}
	if strings.TrimSpace(op.Mode) != "" {
		response["mode"] = op.Mode
	}
	if strings.TrimSpace(op.ContainerID) != "" {
		response["containerId"] = op.ContainerID
	}
	if strings.TrimSpace(op.RuntimeState) != "" {
		response["runtimeState"] = op.RuntimeState
	}
	if op.EndedAt != nil {
		response["endedAt"] = op.EndedAt.Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(op.ErrorMessage) != "" {
		code := strings.TrimSpace(op.ErrorCode)
		if code == "" {
			code = ErrCodeInternal
		}
		response["error"] = map[string]any{
			"code":    code,
			"message": op.ErrorMessage,
		}
	}
	return response
}

func snapshotControlPlaneApplyAccepted(op *controlPlaneApplyOperation) map[string]any {
	return map[string]any{
		"operationId":    op.OperationID,
		"status":         op.Status,
		"controllerUuid": op.ControllerUUID,
		"generation":     op.Generation,
		"namespace":      op.Namespace,
		"name":           op.Name,
		"image":          op.Image,
		"startedAt":      op.StartedAt.Format(time.RFC3339Nano),
	}
}

func snapshotRuntimeClassApplyOperation(op *runtimeClassApplyOperation) map[string]any {
	return runtimeClassApplyOperationResponse(op)
}

func snapshotRuntimeClassDeleteOperation(op *runtimeClassDeleteOperation) map[string]any {
	return runtimeClassDeleteOperationResponse(op)
}
