package scannode

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func newAILocalModelOperation(
	operationID string,
	kind string,
	modelName string,
	title string,
	targetNodeID string,
) *aiv1.AILocalModelOperation {
	return &aiv1.AILocalModelOperation{
		OperationId: strings.TrimSpace(operationID),
		Kind:        strings.TrimSpace(kind),
		ModelName:   strings.TrimSpace(modelName),
		Title:       strings.TrimSpace(title),
		TargetNodeId: strings.TrimSpace(targetNodeID),
	}
}

func (b *legionJobBridge) runAILlamaServerInstallOperation(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	proxy string,
) {
	defer b.aiLocalModelOps.Remove(operation.GetOperationId())
	stream := newLocalModelExecStream(ctx, func(result *ypb.ExecResult) {
		b.publishAILocalModelOperationProgress(ctx, ref, operation, result)
	})
	if err := newLocalModelGRPCServer().InstallLlamaServer(&ypb.InstallLlamaServerRequest{
		Proxy: proxy,
	}, stream); err != nil {
		b.publishAILocalModelOperationFailure(ctx, ref, operation, err)
		return
	}
	response, err := newLocalModelGRPCServer().IsLlamaServerReady(ctx, &ypb.Empty{})
	if err != nil {
		b.publishAILocalModelOperationFailure(ctx, ref, operation, err)
		return
	}
	if err := b.ensureAIPublisher().PublishAILocalModelOperationCompleted(ctx, ref, operation, response.GetReason(), nil); err != nil {
		return
	}
	_ = b.ensureAIPublisher().PublishAILlamaServerInstalled(ctx, ref, response.GetOk(), response.GetReason())
}

func (b *legionJobBridge) runAILocalModelDownloadOperation(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	modelName string,
	proxy string,
) {
	defer b.aiLocalModelOps.Remove(operation.GetOperationId())
	stream := newLocalModelExecStream(ctx, func(result *ypb.ExecResult) {
		b.publishAILocalModelOperationProgress(ctx, ref, operation, result)
	})
	if err := newLocalModelGRPCServer().DownloadLocalModel(&ypb.DownloadLocalModelRequest{
		ModelName: modelName,
		Proxy:     proxy,
	}, stream); err != nil {
		b.publishAILocalModelOperationFailure(ctx, ref, operation, err)
		return
	}
	item, err := findAILocalModelByName(ctx, modelName)
	if err != nil {
		b.publishAILocalModelOperationFailure(ctx, ref, operation, err)
		return
	}
	if err := b.ensureAIPublisher().PublishAILocalModelOperationCompleted(ctx, ref, operation, "ok", item); err != nil {
		return
	}
	_ = b.ensureAIPublisher().PublishAILocalModelDownloaded(ctx, ref, item, "ok")
}

func (b *legionJobBridge) runAILocalModelStartOperation(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	command *aiv1.StartAILocalModelCommand,
) {
	defer b.aiLocalModelOps.Remove(operation.GetOperationId())
	stream := newLocalModelExecStream(ctx, func(result *ypb.ExecResult) {
		b.publishAILocalModelOperationProgress(ctx, ref, operation, result)
	})
	if err := newLocalModelGRPCServer().StartLocalModel(&ypb.StartLocalModelRequest{
		ModelName:        strings.TrimSpace(command.GetModelName()),
		Host:             strings.TrimSpace(command.GetHost()),
		Port:             command.GetPort(),
		ContextSize:      command.GetContextSize(),
		BatchSize:        command.GetBatchSize(),
		Threads:          command.GetThreads(),
		Debug:            command.GetDebug(),
		Pooling:          strings.TrimSpace(command.GetPooling()),
		StartupTimeoutMs: command.GetStartupTimeoutMs(),
		Args:             append([]string(nil), command.GetArgs()...),
	}, stream); err != nil {
		b.publishAILocalModelOperationFailure(ctx, ref, operation, err)
		return
	}
	item, err := findAILocalModelByName(ctx, command.GetModelName())
	if err != nil {
		b.publishAILocalModelOperationFailure(ctx, ref, operation, err)
		return
	}
	if err := b.ensureAIPublisher().PublishAILocalModelOperationCompleted(ctx, ref, operation, "ok", item); err != nil {
		return
	}
	_ = b.ensureAIPublisher().PublishAILocalModelStarted(ctx, ref, item, "ok")
}

func (b *legionJobBridge) publishAILocalModelOperationProgress(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	result *ypb.ExecResult,
) {
	if result == nil {
		return
	}
	message := ""
	if result.GetIsMessage() {
		message = strings.TrimSpace(string(result.GetMessage()))
	}
	_ = b.ensureAIPublisher().PublishAILocalModelOperationProgressed(
		ctx,
		ref,
		operation,
		result.GetProgress(),
		message,
		string(result.GetMessage()),
	)
}

func (b *legionJobBridge) publishAILocalModelOperationFailure(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	err error,
) {
	if err == nil {
		return
	}
	if ctx.Err() != nil {
		_ = b.ensureAIPublisher().PublishAILocalModelOperationCancelled(ctx, ref, operation, "cancelled")
		return
	}
	_ = b.ensureAIPublisher().PublishAILocalModelOperationFailed(
		ctx,
		ref,
		operation,
		classifyAILocalModelError(err),
		err.Error(),
	)
}

func (b *legionJobBridge) handleAILocalModelsList(ctx context.Context, raw []byte) error {
	var command aiv1.ListAILocalModelsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local models list command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local models list", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelsListFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}

	items, err := listAILocalModels(ctx)
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelsListFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAILocalModelsListed(ctx, ref, items)
}

func (b *legionJobBridge) handleAILlamaServerReady(ctx context.Context, raw []byte) error {
	var command aiv1.CheckAILlamaServerReadyCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai llama server ready command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai llama server ready", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILlamaServerReadyCheckFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}

	response, err := newLocalModelGRPCServer().IsLlamaServerReady(ctx, &ypb.Empty{})
	if err != nil {
		return b.ensureAIPublisher().PublishAILlamaServerReadyCheckFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAILlamaServerReadyChecked(ctx, ref, response.GetOk(), response.GetReason())
}

func (b *legionJobBridge) handleAILlamaServerInstall(ctx context.Context, raw []byte) error {
	var command aiv1.InstallAILlamaServerCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai llama server install command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai llama server install", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILlamaServerInstallFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}
	operation := newAILocalModelOperation(command.GetMetadata().GetCommandId(), "install_llama_server", "", "安装 llama-server", command.GetTargetNodeId())
	opCtx, cancel := context.WithCancel(context.Background())
	b.aiLocalModelOps.Store(operation.GetOperationId(), cancel)
	if err := b.ensureAIPublisher().PublishAILocalModelOperationAccepted(ctx, ref, operation); err != nil {
		b.aiLocalModelOps.Remove(operation.GetOperationId())
		cancel()
		return err
	}
	go b.runAILlamaServerInstallOperation(opCtx, ref, operation, strings.TrimSpace(command.GetProxy()))
	return nil
}

func (b *legionJobBridge) handleAILocalModelCreate(ctx context.Context, raw []byte) error {
	var command aiv1.CreateAILocalModelCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local model create command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local model create", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelCreateFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}
	if strings.TrimSpace(command.GetName()) == "" {
		return b.ensureAIPublisher().PublishAILocalModelCreateFailed(ctx, ref, "ai_local_model_invalid_name", "model name is required")
	}
	if strings.TrimSpace(command.GetPath()) == "" {
		return b.ensureAIPublisher().PublishAILocalModelCreateFailed(ctx, ref, "ai_local_model_invalid_path", "model path is required")
	}

	response, err := newLocalModelGRPCServer().AddLocalModel(ctx, &ypb.AddLocalModelRequest{
		Name:        strings.TrimSpace(command.GetName()),
		ModelType:   strings.TrimSpace(command.GetModelType()),
		Description: strings.TrimSpace(command.GetDescription()),
		Path:        strings.TrimSpace(command.GetPath()),
	})
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelCreateFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	if err := ensureLocalModelGeneralResponse(response); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelCreateFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}

	item, err := findAILocalModelByName(ctx, command.GetName())
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelCreateFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAILocalModelCreated(ctx, ref, item, "ok")
}

func (b *legionJobBridge) handleAILocalModelUpdate(ctx context.Context, raw []byte) error {
	var command aiv1.UpdateAILocalModelCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local model update command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local model update", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelUpdateFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}
	if strings.TrimSpace(command.GetModelName()) == "" {
		return b.ensureAIPublisher().PublishAILocalModelUpdateFailed(ctx, ref, "ai_local_model_invalid_name", "model name is required")
	}

	response, err := newLocalModelGRPCServer().UpdateLocalModel(ctx, &ypb.UpdateLocalModelRequest{
		Name:        strings.TrimSpace(command.GetModelName()),
		ModelType:   strings.TrimSpace(command.GetModelType()),
		Description: strings.TrimSpace(command.GetDescription()),
		Path:        strings.TrimSpace(command.GetPath()),
	})
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelUpdateFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	if err := ensureLocalModelGeneralResponse(response); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelUpdateFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}

	item, err := findAILocalModelByName(ctx, command.GetModelName())
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelUpdateFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAILocalModelUpdated(ctx, ref, item, "ok")
}

func (b *legionJobBridge) handleAILocalModelDelete(ctx context.Context, raw []byte) error {
	var command aiv1.DeleteAILocalModelCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local model delete command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local model delete", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelDeleteFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}
	modelName := strings.TrimSpace(command.GetModelName())
	if modelName == "" {
		return b.ensureAIPublisher().PublishAILocalModelDeleteFailed(ctx, ref, "ai_local_model_invalid_name", "model name is required")
	}

	item, err := findAILocalModelByName(ctx, modelName)
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelDeleteFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	response, err := newLocalModelGRPCServer().DeleteLocalModel(ctx, &ypb.DeleteLocalModelRequest{
		Name:             modelName,
		DeleteSourceFile: command.GetDeleteSourceFile(),
	})
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelDeleteFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	if err := ensureLocalModelGeneralResponse(response); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelDeleteFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAILocalModelDeleted(ctx, ref, item, "ok")
}

func (b *legionJobBridge) handleAILocalModelStart(ctx context.Context, raw []byte) error {
	var command aiv1.StartAILocalModelCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local model start command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local model start", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelStartFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}
	modelName := strings.TrimSpace(command.GetModelName())
	if modelName == "" {
		return b.ensureAIPublisher().PublishAILocalModelStartFailed(ctx, ref, "ai_local_model_invalid_name", "model name is required")
	}
	operation := newAILocalModelOperation(command.GetMetadata().GetCommandId(), "start_model", modelName, fmt.Sprintf("启动模型 %s", modelName), command.GetTargetNodeId())
	opCtx, cancel := context.WithCancel(context.Background())
	b.aiLocalModelOps.Store(operation.GetOperationId(), cancel)
	if err := b.ensureAIPublisher().PublishAILocalModelOperationAccepted(ctx, ref, operation); err != nil {
		b.aiLocalModelOps.Remove(operation.GetOperationId())
		cancel()
		return err
	}
	go b.runAILocalModelStartOperation(opCtx, ref, operation, &command)
	return nil
}

func (b *legionJobBridge) handleAILocalModelStop(ctx context.Context, raw []byte) error {
	var command aiv1.StopAILocalModelCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local model stop command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local model stop", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelStopFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}
	modelName := strings.TrimSpace(command.GetModelName())
	if modelName == "" {
		return b.ensureAIPublisher().PublishAILocalModelStopFailed(ctx, ref, "ai_local_model_invalid_name", "model name is required")
	}

	response, err := newLocalModelGRPCServer().StopLocalModel(ctx, &ypb.StopLocalModelRequest{
		ModelName: modelName,
	})
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelStopFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	if err := ensureLocalModelGeneralResponse(response); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelStopFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}

	item, err := findAILocalModelByName(ctx, modelName)
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelStopFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAILocalModelStopped(ctx, ref, item, "ok")
}

func (b *legionJobBridge) handleAILocalModelDownload(ctx context.Context, raw []byte) error {
	var command aiv1.DownloadAILocalModelCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local model download command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local model download", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelDownloadFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}
	modelName := strings.TrimSpace(command.GetModelName())
	if modelName == "" {
		return b.ensureAIPublisher().PublishAILocalModelDownloadFailed(ctx, ref, "ai_local_model_invalid_name", "model name is required")
	}
	operation := newAILocalModelOperation(command.GetMetadata().GetCommandId(), "download_model", modelName, fmt.Sprintf("下载模型 %s", modelName), command.GetTargetNodeId())
	opCtx, cancel := context.WithCancel(context.Background())
	b.aiLocalModelOps.Store(operation.GetOperationId(), cancel)
	if err := b.ensureAIPublisher().PublishAILocalModelOperationAccepted(ctx, ref, operation); err != nil {
		b.aiLocalModelOps.Remove(operation.GetOperationId())
		cancel()
		return err
	}
	go b.runAILocalModelDownloadOperation(opCtx, ref, operation, modelName, strings.TrimSpace(command.GetProxy()))
	return nil
}

func (b *legionJobBridge) handleAILocalModelOperationCancel(ctx context.Context, raw []byte) error {
	var command aiv1.CancelAILocalModelOperationCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local model operation cancel command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local model operation cancel", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelOperationFailed(
			ctx,
			ref,
			newAILocalModelOperation(strings.TrimSpace(command.GetOperationId()), "", "", "", command.GetTargetNodeId()),
			"ai_local_model_invalid_command",
			err.Error(),
		)
	}

	operationID := strings.TrimSpace(command.GetOperationId())
	if operationID == "" {
		return b.ensureAIPublisher().PublishAILocalModelOperationFailed(
			ctx,
			ref,
			newAILocalModelOperation(operationID, "", "", "", command.GetTargetNodeId()),
			"ai_local_model_invalid_operation",
			"operation id is required",
		)
	}

	if !b.aiLocalModelOps.Cancel(operationID) {
		return b.ensureAIPublisher().PublishAILocalModelOperationFailed(
			ctx,
			ref,
			newAILocalModelOperation(operationID, "", "", "", command.GetTargetNodeId()),
			"ai_local_model_operation_not_found",
			"operation not found",
		)
	}
	return nil
}

func (b *legionJobBridge) handleAILocalModelsClear(ctx context.Context, raw []byte) error {
	var command aiv1.ClearAILocalModelsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai local models clear command: %w", err)
	}

	ref := aiLocalModelRef(command.GetMetadata(), command.GetOwnerUserId())
	if err := validateAILocalModelCommand(b.agent.node.CurrentNodeID(), "ai local models clear", command.GetMetadata(), command.GetTargetNodeId(), command.GetOwnerUserId()); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelsClearFailed(ctx, ref, "ai_local_model_invalid_command", err.Error())
	}

	response, err := newLocalModelGRPCServer().ClearAllModels(ctx, &ypb.ClearAllModelsRequest{
		DeleteSourceFile: command.GetDeleteSourceFile(),
	})
	if err != nil {
		return b.ensureAIPublisher().PublishAILocalModelsClearFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	if err := ensureLocalModelGeneralResponse(response); err != nil {
		return b.ensureAIPublisher().PublishAILocalModelsClearFailed(
			ctx,
			ref,
			classifyAILocalModelError(err),
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAILocalModelsCleared(ctx, ref, response.GetOk(), response.GetReason())
}

func validateAILocalModelCommand(
	nodeID string,
	label string,
	metadataMessage interface{ GetCommandId() string },
	targetNodeID string,
	ownerUserID string,
) error {
	switch {
	case metadataMessage == nil:
		return fmt.Errorf("%s metadata is required", label)
	case strings.TrimSpace(metadataMessage.GetCommandId()) == "":
		return fmt.Errorf("%s command_id is required", label)
	case strings.TrimSpace(targetNodeID) == "":
		return fmt.Errorf("%s target_node_id is required", label)
	case strings.TrimSpace(targetNodeID) != nodeID:
		return fmt.Errorf("%s target_node_id mismatch: %s", label, targetNodeID)
	case strings.TrimSpace(ownerUserID) == "":
		return fmt.Errorf("%s owner_user_id is required", label)
	default:
		return nil
	}
}

func aiLocalModelRef(metadata interface{ GetCommandId() string }, ownerUserID string) aiLocalModelCommandRef {
	commandID := ""
	if metadata != nil {
		commandID = strings.TrimSpace(metadata.GetCommandId())
	}
	return aiLocalModelCommandRef{
		CommandID:   commandID,
		OwnerUserID: strings.TrimSpace(ownerUserID),
	}
}

func listAILocalModels(ctx context.Context) ([]*aiv1.AILocalModelRecord, error) {
	response, err := newLocalModelGRPCServer().GetSupportedLocalModels(ctx, &ypb.Empty{})
	if err != nil {
		return nil, err
	}
	items := make([]*aiv1.AILocalModelRecord, 0, len(response.GetModels()))
	for _, item := range response.GetModels() {
		if item == nil {
			continue
		}
		items = append(items, mapAILocalModelRecord(item))
	}
	return items, nil
}

func findAILocalModelByName(ctx context.Context, modelName string) (*aiv1.AILocalModelRecord, error) {
	items, err := listAILocalModels(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item != nil && item.GetName() == strings.TrimSpace(modelName) {
			return item, nil
		}
	}
	return nil, fmt.Errorf("model not found: %s", strings.TrimSpace(modelName))
}

func ensureLocalModelGeneralResponse(response *ypb.GeneralResponse) error {
	switch {
	case response == nil:
		return fmt.Errorf("local model response is nil")
	case !response.GetOk():
		reason := strings.TrimSpace(response.GetReason())
		if reason == "" {
			reason = "local model operation failed"
		}
		return fmt.Errorf(reason)
	default:
		return nil
	}
}

func classifyAILocalModelError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "名称不能为空"), strings.Contains(message, "model name is required"):
		return "ai_local_model_invalid_name"
	case strings.Contains(message, "模型路径不能为空"),
		strings.Contains(message, "模型文件不存在"),
		strings.Contains(message, "model path is required"):
		return "ai_local_model_invalid_path"
	case strings.Contains(message, "不支持的模型类型"), strings.Contains(message, "invalid model type"):
		return "ai_local_model_invalid_type"
	case strings.Contains(message, "名称已存在"), strings.Contains(message, "already exists"):
		return "ai_local_model_conflict"
	case strings.Contains(message, "模型不存在"), strings.Contains(message, "model not found"), strings.Contains(message, "不支持的模型"):
		return "ai_local_model_not_found"
	case strings.Contains(message, "llama-server"):
		return "ai_local_model_llama_server_unavailable"
	default:
		return "ai_local_model_unavailable"
	}
}

func mapAILocalModelRecord(record *ypb.LocalModelConfig) *aiv1.AILocalModelRecord {
	if record == nil {
		return nil
	}
	return &aiv1.AILocalModelRecord{
		Name:        record.GetName(),
		Type:        record.GetType(),
		FileName:    record.GetFileName(),
		DownloadUrl: record.GetDownloadURL(),
		Description: record.GetDescription(),
		DefaultPort: record.GetDefaultPort(),
		IsLocal:     record.GetIsLocal(),
		IsReady:     record.GetIsReady(),
		Path:        record.GetPath(),
		Status:      mapAILocalModelStatus(record.GetStatus()),
	}
}

func mapAILocalModelStatus(status *ypb.LocalModelStatus) *aiv1.AILocalModelStatus {
	if status == nil {
		return nil
	}
	return &aiv1.AILocalModelStatus{
		Status:               status.GetStatus(),
		Host:                 status.GetHost(),
		Port:                 status.GetPort(),
		Model:                status.GetModel(),
		ModelPath:            status.GetModelPath(),
		LlamaServerPath:      status.GetLlamaServerPath(),
		ContextSize:          status.GetContextSize(),
		ContBatching:         status.GetContBatching(),
		BatchSize:            status.GetBatchSize(),
		Threads:              status.GetThreads(),
		Detached:             status.GetDetached(),
		Debug:                status.GetDebug(),
		Pooling:              status.GetPooling(),
		StartupTimeoutSecond: status.GetStartupTimeout(),
		Args:                 append([]string(nil), status.GetArgs()...),
	}
}

func newLocalModelGRPCServer() *yakgrpc.Server {
	return &yakgrpc.Server{}
}

type localModelExecStream struct {
	ctx     context.Context
	onEvent func(*ypb.ExecResult)
}

func newLocalModelExecStream(ctx context.Context, onEvent func(*ypb.ExecResult)) *localModelExecStream {
	return &localModelExecStream{ctx: ctx, onEvent: onEvent}
}

func (s *localModelExecStream) SetHeader(metadata.MD) error { return nil }
func (s *localModelExecStream) SendHeader(metadata.MD) error { return nil }
func (s *localModelExecStream) SetTrailer(metadata.MD)       {}
func (s *localModelExecStream) Context() context.Context     { return s.ctx }
func (s *localModelExecStream) SendMsg(message interface{}) error {
	if result, ok := message.(*ypb.ExecResult); ok {
		return s.Send(result)
	}
	return nil
}
func (s *localModelExecStream) RecvMsg(interface{}) error { return nil }
func (s *localModelExecStream) Send(result *ypb.ExecResult) error {
	if s.onEvent != nil && result != nil {
		s.onEvent(result)
	}
	return nil
}
