package scannode

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/log"
	commonbundle "github.com/yaklang/yaklang/common/pluginbundle"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

const pluginBundleManifestSchemaVersion = "legion_plugin_bundle.v1"

func (b *legionJobBridge) handleDispatch(
	ctx context.Context,
	raw []byte,
) error {
	var command jobv1.DispatchJobCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal dispatch command: %w", err)
	}

	ref := jobExecutionRefFromCommand(&command)
	if err := validateDispatchCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.publishDispatchFailure(
			ctx,
			ref,
			"invalid_dispatch_command",
			err.Error(),
			&command,
		)
	}

	if err := b.publisher.PublishClaimed(ctx, ref); err != nil {
		return err
	}
	if err := b.publisher.PublishStarted(ctx, ref); err != nil {
		return err
	}
	go b.executeDispatch(ref, &command)
	return nil
}

func (b *legionJobBridge) executeDispatch(
	ref jobExecutionRef,
	command *jobv1.DispatchJobCommand,
) {
	execCtx := withLegionJobExecutionRef(b.agent.node.GetRootContext(), ref)
	response, err := b.agent.executeScriptTask(
		execCtx,
		ScriptExecutionRequest{
			TaskID:          ref.JobID,
			RuntimeID:       ref.AttemptID,
			SubTaskID:       ref.SubtaskID,
			ScriptContent:   command.GetScript().GetContent(),
			ScriptJSONParam: normalizeInputJSON(command.GetInputJson()),
			ScriptLabels:    command.GetLabels(),
			PluginBundle:    command.GetPluginBundle(),
			DebugEnabled:    isDebugEnabled(command.GetLabels()),
			DebugDir:        resolveDebugDir(command.GetLabels()),
		},
	)
	if err == nil {
		if publishErr := b.publisher.PublishSucceeded(
			b.agent.node.GetRootContext(),
			ref,
			response,
		); publishErr != nil {
			logDispatchPublishError("success", publishErr)
		}
		return
	}

	var cancelled *TaskCancelledError
	if errors.As(err, &cancelled) {
		b.publishCancelled(ref, cancelled)
		return
	}
	if publishErr := b.publisher.PublishFailed(
		b.agent.node.GetRootContext(),
		ref,
		"script_execution_failed",
		err.Error(),
		dispatchFailureDetail(command),
	); publishErr != nil {
		logDispatchPublishError("failed", publishErr)
	}
}

func (b *legionJobBridge) handleCancel(raw []byte) error {
	var command jobv1.CancelJobCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal cancel command: %w", err)
	}
	subtaskID := command.GetJob().GetSubtaskId()
	if strings.TrimSpace(subtaskID) == "" {
		return fmt.Errorf("cancel command subtask_id is required")
	}

	task, err := b.agent.manager.GetTaskById(taskIDForSubtask(subtaskID))
	if err != nil {
		logCancelTargetMissing(subtaskID)
		return nil
	}
	reason := strings.TrimSpace(command.GetReason())
	if reason == "" {
		reason = "platform cancel requested"
	}
	task.SetCancelReason(reason)
	task.MarkCancelRequested()
	task.Cancel()
	return nil
}

func validateDispatchCommand(
	nodeID string,
	command *jobv1.DispatchJobCommand,
) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("dispatch metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("dispatch command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("dispatch target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("dispatch target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetJob() == nil:
		return fmt.Errorf("dispatch job reference is required")
	case strings.TrimSpace(command.GetJob().GetJobId()) == "":
		return fmt.Errorf("dispatch job_id is required")
	case strings.TrimSpace(command.GetJob().GetSubtaskId()) == "":
		return fmt.Errorf("dispatch subtask_id is required")
	case strings.TrimSpace(command.GetJob().GetAttemptId()) == "":
		return fmt.Errorf("dispatch attempt_id is required")
	case command.GetScript() == nil:
		return fmt.Errorf("dispatch script is required")
	case command.GetScript().GetVersion() == nil:
		return fmt.Errorf("dispatch script version is required")
	case strings.TrimSpace(command.GetScript().GetVersion().GetReleaseId()) == "":
		return fmt.Errorf("dispatch script release_id is required")
	case strings.TrimSpace(command.GetScript().GetContent()) == "":
		return fmt.Errorf("dispatch script content is required")
	default:
		if err := validateDispatchExecutionKind(command.GetExecutionKind()); err != nil {
			return err
		}
		return validatePluginBundleRef(command.GetPluginBundle())
	}
}

func validatePluginBundleRef(ref *pluginv1.PluginBundleRef) error {
	if ref == nil {
		return nil
	}
	if strings.TrimSpace(ref.GetBundleId()) == "" {
		return fmt.Errorf("plugin bundle_id is required")
	}
	digest, err := hex.DecodeString(strings.TrimSpace(ref.GetArtifactSha256()))
	if err != nil || len(digest) != 32 {
		return fmt.Errorf("plugin bundle artifact_sha256 is invalid")
	}
	if ref.GetArtifactSizeBytes() <= 0 || ref.GetArtifactSizeBytes() > commonbundle.MaxArtifactSize {
		return fmt.Errorf("plugin bundle artifact_size_bytes is outside the allowed range")
	}
	if ref.GetItemCount() <= 0 || ref.GetItemCount() > commonbundle.MaxMemberCount {
		return fmt.Errorf("plugin bundle item_count is outside the allowed range")
	}
	if strings.TrimSpace(ref.GetArchiveFormat()) != "zip" {
		return fmt.Errorf("unsupported plugin bundle archive_format: %s", ref.GetArchiveFormat())
	}
	if strings.TrimSpace(ref.GetSchemaVersion()) != pluginBundleManifestSchemaVersion {
		return fmt.Errorf("unsupported plugin bundle schema_version: %s", ref.GetSchemaVersion())
	}
	return nil
}

func (b *legionJobBridge) publishDispatchFailure(
	ctx context.Context,
	ref jobExecutionRef,
	code string,
	message string,
	command *jobv1.DispatchJobCommand,
) error {
	return b.publisher.PublishFailed(ctx, ref, code, message, dispatchFailureDetail(command))
}

func (b *legionJobBridge) publishCancelled(
	ref jobExecutionRef,
	cancelled *TaskCancelledError,
) {
	reason := cancelled.Reason
	if reason == "" {
		reason = "cancel requested"
	}
	if publishErr := b.publisher.PublishCancelled(
		b.agent.node.GetRootContext(),
		ref,
		reason,
	); publishErr != nil {
		logDispatchPublishError("cancelled", publishErr)
	}
}

func validateDispatchExecutionKind(value string) error {
	if strings.TrimSpace(value) != "yak_script" {
		return fmt.Errorf("unsupported execution_kind: %s", value)
	}
	return nil
}

func normalizeInputJSON(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "{}"
	}
	return trimmed
}

func jobExecutionRefFromCommand(command *jobv1.DispatchJobCommand) jobExecutionRef {
	return jobExecutionRef{
		CommandID: command.GetMetadata().GetCommandId(),
		JobID:     command.GetJob().GetJobId(),
		SubtaskID: command.GetJob().GetSubtaskId(),
		AttemptID: command.GetJob().GetAttemptId(),
	}
}

func dispatchFailureDetail(command *jobv1.DispatchJobCommand) map[string]string {
	scriptVersion := ""
	if command.GetScript() != nil && command.GetScript().GetVersion() != nil {
		scriptVersion = command.GetScript().GetVersion().GetReleaseId()
	}
	return map[string]string{
		"script_release_id": scriptVersion,
		"execution_kind":    command.GetExecutionKind(),
	}
}

func logDispatchPublishError(kind string, err error) {
	log.Errorf("publish legion %s event failed: %v", kind, err)
}

func logCancelTargetMissing(subtaskID string) {
	log.Warnf("cancel command target not found: subtask_id=%s", subtaskID)
}
