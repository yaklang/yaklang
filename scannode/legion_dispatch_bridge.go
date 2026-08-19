package scannode

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/log"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
)

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
			DebugEnabled:    isDebugEnabled(command.GetLabels()),
			DebugDir:        resolveDebugDir(command.GetLabels()),
			RuleSnapshot:    ruleSnapshotExpectationFromCommand(command),
			RuleSnapshotPrepared: func(ctx context.Context, receipt RuleSnapshotPreparationReceipt) error {
				return b.publisher.PublishRuleSnapshotPrepared(ctx, ref, receipt)
			},
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
	failureCode := "script_execution_failed"
	failureDetail := dispatchFailureDetail(command)
	var preparationErr *ruleSnapshotPreparationError
	if errors.As(err, &preparationErr) {
		failureCode = "rule_snapshot_prepare_failed"
		if preparationErr.Expectation.SnapshotID != "" {
			failureDetail["rule_snapshot_id"] = preparationErr.Expectation.SnapshotID
		}
		if preparationErr.Expectation.ContentSHA256 != "" {
			failureDetail["rule_snapshot_content_sha256"] = preparationErr.Expectation.ContentSHA256
		}
	}
	if publishErr := b.publisher.PublishFailed(
		b.agent.node.GetRootContext(),
		ref,
		failureCode,
		err.Error(),
		failureDetail,
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
	}
	if err := validateRuleSnapshotRef(command.GetRuleSnapshot()); err != nil {
		return err
	}
	return validateDispatchExecutionKind(command.GetExecutionKind())
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
	detail := map[string]string{
		"script_release_id": scriptVersion,
		"execution_kind":    command.GetExecutionKind(),
	}
	if snapshot := command.GetRuleSnapshot(); snapshot != nil {
		detail["rule_snapshot_id"] = snapshot.GetSnapshotId()
		detail["rule_snapshot_content_sha256"] = snapshot.GetContentSha256()
		detail["rule_snapshot_schema_version"] = snapshot.GetSchemaVersion()
		detail["rule_snapshot_bundle_format"] = snapshot.GetBundleFormat()
	}
	return detail
}

func validateRuleSnapshotRef(ref *jobv1.RuleSnapshotRef) error {
	if ref == nil {
		return nil
	}
	switch {
	case strings.TrimSpace(ref.GetSnapshotId()) == "":
		return fmt.Errorf("dispatch rule_snapshot snapshot_id is required")
	case strings.TrimSpace(ref.GetContentSha256()) == "":
		return fmt.Errorf("dispatch rule_snapshot content_sha256 is required")
	case strings.TrimSpace(ref.GetSchemaVersion()) == "":
		return fmt.Errorf("dispatch rule_snapshot schema_version is required")
	case strings.TrimSpace(ref.GetBundleFormat()) == "":
		return fmt.Errorf("dispatch rule_snapshot bundle_format is required")
	case len(ref.GetAssetIds()) == 0:
		return fmt.Errorf("dispatch rule_snapshot asset_ids are required")
	}
	_, err := normalizeRuleSnapshotExpectation(RuleSnapshotExpectation{
		SnapshotID:    ref.GetSnapshotId(),
		ContentSHA256: ref.GetContentSha256(),
		SchemaVersion: ref.GetSchemaVersion(),
		BundleFormat:  ref.GetBundleFormat(),
		AssetIDs:      append([]string(nil), ref.GetAssetIds()...),
	})
	return err
}

func ruleSnapshotExpectationFromCommand(
	command *jobv1.DispatchJobCommand,
) *RuleSnapshotExpectation {
	if command == nil || command.GetRuleSnapshot() == nil {
		return nil
	}
	ref := command.GetRuleSnapshot()
	return &RuleSnapshotExpectation{
		SnapshotID:    strings.TrimSpace(ref.GetSnapshotId()),
		ContentSHA256: strings.TrimSpace(ref.GetContentSha256()),
		SchemaVersion: strings.TrimSpace(ref.GetSchemaVersion()),
		BundleFormat:  strings.TrimSpace(ref.GetBundleFormat()),
		AssetIDs:      append([]string(nil), ref.GetAssetIds()...),
	}
}

func logDispatchPublishError(kind string, err error) {
	log.Errorf("publish legion %s event failed: %v", kind, err)
}

func logCancelTargetMissing(subtaskID string) {
	log.Warnf("cancel command target not found: subtask_id=%s", subtaskID)
}
