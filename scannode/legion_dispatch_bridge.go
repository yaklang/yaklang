package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/log"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
)

const dispatchCapacityNakDelay = time.Second

var (
	dispatchEventRetryAttempts = 3
	dispatchEventRetryDelay    = 100 * time.Millisecond
)

func (b *legionJobBridge) handleDispatch(
	ctx context.Context,
	sessionID string,
	raw []byte,
) (messageDisposition, error) {
	var command jobv1.DispatchJobCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return termMessage(), fmt.Errorf("unmarshal dispatch command: %w", err)
	}

	ref := jobExecutionRefFromCommand(&command)
	if err := validateDispatchCommand(b.currentNodeID(), &command); err != nil {
		return termMessage(), b.publishDispatchFailure(
			ctx,
			ref,
			JobFailureCodeInvalidDispatchCommand,
			err.Error(),
			&command,
		)
	}
	identity, err := dispatchCommandIdentity(&command)
	if err != nil {
		return termMessage(), err
	}
	now := time.Now().UTC()
	deadline := dispatchAdmissionDeadline(&command, b.heartbeatInterval(), now)
	reservation, reserveResult := b.admissions().Reserve(sessionID, ref, identity, deadline)
	switch reserveResult {
	case dispatchConflict, dispatchStaleSession:
		return termMessage(), nil
	case dispatchCancelled:
		return b.publishPendingCancellation(ctx, reservation)
	case dispatchDuplicate:
		if reason, published := b.admissions().PendingCancel(reservation); reason != "" {
			if published {
				return ackSyncMessage(nil), nil
			}
			return b.publishPendingCancellation(ctx, reservation)
		}
		state, preparing, _, stale := b.admissions().Snapshot(reservation)
		if stale {
			return termMessage(), nil
		}
		switch state {
		case dispatchAdmissionTerminal, dispatchAdmissionRunning:
			return ackSyncMessage(nil), nil
		case dispatchAdmissionClaimed:
			return b.dispatchAckDisposition(reservation, &command), nil
		case dispatchAdmissionPending:
			// The issued-at deadline bounds only the full-capacity retry path.
			// A first delivery may legitimately arrive later because it waited in
			// Legion's outbox or JetStream and must still run when a slot is free.
			if b.admissions().CapacityRetryExpired(reservation, now) {
				return b.publishCapacityExceeded(ctx, reservation, &command)
			}
			if preparing {
				return nakDelayedMessage(dispatchCapacityNakDelay), nil
			}
		}
	}
	if b.admissions().Prepared(reservation) {
		return b.publishClaimedForReservation(ctx, reservation, &command)
	}
	if !b.admissions().BeginPrepare(reservation) {
		return nakDelayedMessage(dispatchCapacityNakDelay), nil
	}
	release, acquired := b.agent.invokeLimiter.TryAcquire()
	if !acquired {
		b.admissions().PrepareFailed(reservation)
		if b.admissions().CapacityRetryExpired(reservation, time.Now().UTC()) {
			return b.publishCapacityExceeded(ctx, reservation, &command)
		}
		return nakDelayedMessage(dispatchCapacityNakDelay), nil
	}

	execCtx := withLegionJobExecutionRef(b.rootContext(), ref)
	taskCtx, cancel := context.WithCancel(execCtx)
	task := newScriptTask(
		taskCtx,
		cancel,
		taskIDForSubtask(ref.SubtaskID),
		ref.JobID,
		ref.SubtaskID,
		ref.AttemptID,
	)
	_, loaded, accepted := b.agent.manager.LoadOrStoreAttempt(task)
	if !accepted || loaded {
		cancel()
		release()
		b.admissions().MarkExpired(reservation)
		return termMessage(), nil
	}
	if !b.admissions().AttachClaim(reservation, task, release) {
		cancel()
		release()
		b.agent.manager.RemoveAttempt(task.AttemptID)
		_, _, _, stale := b.admissions().Snapshot(reservation)
		if stale {
			return termMessage(), nil
		}
		return ackSyncMessage(nil), nil
	}
	return b.publishClaimedForReservation(ctx, reservation, &command)
}

func (b *legionJobBridge) publishClaimedForReservation(
	ctx context.Context,
	reservation *dispatchReservation,
	command *jobv1.DispatchJobCommand,
) (messageDisposition, error) {
	_, _, claimedPublished, stale := b.admissions().Snapshot(reservation)
	if stale {
		return termMessage(), nil
	}
	if !claimedPublished {
		if err := b.dispatchReporter().PublishClaimed(ctx, reservation.ref); err != nil {
			b.rollbackPreparedReservation(reservation)
			return nakMessage(), err
		}
	}
	if !b.admissions().MarkClaimedPublished(reservation) {
		_, _, _, stale := b.admissions().Snapshot(reservation)
		if stale {
			return termMessage(), nil
		}
		return ackSyncMessage(nil), nil
	}
	return b.dispatchAckDisposition(reservation, command), nil
}

func (b *legionJobBridge) dispatchAckDisposition(
	reservation *dispatchReservation,
	command *jobv1.DispatchJobCommand,
) messageDisposition {
	return ackSyncDispatchMessage(
		func() { b.startDispatch(reservation, command) },
		func() { b.rollbackClaimedAcknowledgement(reservation) },
	)
}

func (b *legionJobBridge) rollbackClaimedAcknowledgement(reservation *dispatchReservation) {
	task, release, rolledBack := b.admissions().RollbackClaimedAck(reservation)
	if !rolledBack {
		return
	}
	if task != nil {
		task.Cancel()
		b.agent.manager.RemoveAttempt(task.AttemptID)
	}
	if release != nil {
		release()
	}
}

func (b *legionJobBridge) publishPendingCancellation(
	ctx context.Context,
	reservation *dispatchReservation,
) (messageDisposition, error) {
	reason, published := b.admissions().PendingCancel(reservation)
	if published {
		return ackSyncMessage(nil), nil
	}
	if err := b.dispatchReporter().PublishCancelled(ctx, reservation.ref, reason); err != nil {
		return nakMessage(), err
	}
	b.admissions().MarkPendingCancelPublished(reservation)
	b.admissions().CompactTerminal(reservation)
	return ackSyncMessage(nil), nil
}

// publishCapacityExceeded reports a capacity drop to Legion instead of
// terminating the message silently, so the platform records the real failure
// cause rather than reclaiming the attempt later as
// attempt_missing_from_heartbeat. The reservation is expired only after the
// report succeeds; a publish failure NAKs while the reservation stays pending,
// so the next redelivery retries the report instead of acking it away.
func (b *legionJobBridge) publishCapacityExceeded(
	ctx context.Context,
	reservation *dispatchReservation,
	command *jobv1.DispatchJobCommand,
) (messageDisposition, error) {
	err := b.dispatchReporter().PublishFailed(
		ctx,
		reservation.ref,
		JobFailureCodeNodeCapacityExceeded,
		"node execution slots are full and the dispatch admission deadline expired",
		dispatchFailureDetail(command),
	)
	if err != nil {
		return nakMessage(), err
	}
	b.expirePendingReservation(reservation)
	return termMessage(), nil
}

func (b *legionJobBridge) rollbackPreparedReservation(reservation *dispatchReservation) {
	task, release, detached := b.admissions().DetachPrepared(reservation)
	if !detached {
		return
	}
	if task != nil {
		task.Cancel()
		b.agent.manager.RemoveAttempt(task.AttemptID)
	}
	if release != nil {
		release()
	}
}

func (b *legionJobBridge) expirePendingReservation(reservation *dispatchReservation) {
	b.admissions().MarkExpired(reservation)
	task, release := b.admissions().TaskAndRelease(reservation)
	if task != nil {
		task.Cancel()
		b.agent.manager.RemoveAttempt(task.AttemptID)
	}
	if release != nil {
		release()
	}
	b.admissions().CompactTerminal(reservation)
}

func (b *legionJobBridge) startDispatch(
	reservation *dispatchReservation,
	command *jobv1.DispatchJobCommand,
) {
	task, start := b.admissions().MarkRunning(reservation)
	if !start || task == nil {
		return
	}
	task.MarkRunning()
	command = proto.Clone(command).(*jobv1.DispatchJobCommand)
	go func() {
		if err := retryDispatchEvent(func(ctx context.Context) error {
			return b.dispatchReporter().PublishStarted(ctx, reservation.ref)
		}); err != nil {
			b.finishDispatch(reservation, task, func(ctx context.Context) error {
				return b.dispatchReporter().PublishFailed(
					ctx,
					reservation.ref,
					JobFailureCodeStartedEventPublishFailed,
					err.Error(),
					dispatchFailureDetail(command),
				)
			})
			return
		}
		b.executeDispatch(reservation, task, command)
	}()
}

func (b *legionJobBridge) executeDispatch(
	reservation *dispatchReservation,
	task *Task,
	command *jobv1.DispatchJobCommand,
) {
	finished := false
	defer func() {
		if recovered := recover(); recovered != nil && !finished {
			b.finishDispatch(reservation, task, func(ctx context.Context) error {
				return b.dispatchReporter().PublishFailed(
					ctx,
					reservation.ref,
					JobFailureCodeScriptExecutionPanic,
					fmt.Sprintf("panic: %v", recovered),
					map[string]string{"stack": string(debug.Stack())},
				)
			})
		}
	}()

	executor := b.dispatchExecutor
	if executor == nil {
		executor = b.agent.executeScriptTask
	}
	response, err := executor(
		task,
		ScriptExecutionRequest{
			TaskID:          reservation.ref.JobID,
			RuntimeID:       reservation.ref.AttemptID,
			SubTaskID:       reservation.ref.SubtaskID,
			ScriptContent:   command.GetScript().GetContent(),
			ScriptJSONParam: normalizeInputJSON(command.GetInputJson()),
			ScriptLabels:    command.GetLabels(),
			DebugEnabled:    isDebugEnabled(command.GetLabels()),
			DebugDir:        resolveDebugDir(command.GetLabels()),
			RuleSnapshot:    ruleSnapshotExpectationFromCommand(command),
			RuleSnapshotPrepared: func(ctx context.Context, receipt RuleSnapshotPreparationReceipt) error {
				return b.publisher.PublishRuleSnapshotPrepared(ctx, reservation.ref, receipt)
			},
		},
	)
	if err == nil {
		finished = true
		b.finishDispatch(reservation, task, func(ctx context.Context) error {
			return b.dispatchReporter().PublishSucceeded(ctx, reservation.ref, response)
		})
		return
	}

	var cancelled *TaskCancelledError
	if errors.As(err, &cancelled) {
		finished = true
		reason := cancelled.Reason
		if reason == "" {
			reason = task.CancelReason()
		}
		if reason == "" {
			reason = "cancel requested"
		}
		b.finishDispatch(reservation, task, func(ctx context.Context) error {
			return b.dispatchReporter().PublishCancelled(ctx, reservation.ref, reason)
		})
		return
	}
	finished = true
	failureCode := JobFailureCodeScriptExecutionFailed
	failureDetail := dispatchFailureDetail(command)
	var preparationErr *ruleSnapshotPreparationError
	if errors.As(err, &preparationErr) {
		failureCode = JobFailureCodeRuleSnapshotPrepareFailed
		if preparationErr.Expectation.SnapshotID != "" {
			failureDetail["rule_snapshot_id"] = preparationErr.Expectation.SnapshotID
		}
		if preparationErr.Expectation.ContentSHA256 != "" {
			failureDetail["rule_snapshot_content_sha256"] = preparationErr.Expectation.ContentSHA256
		}
	}
	var scriptFail *scriptFailureError
	if errors.As(err, &scriptFail) && strings.TrimSpace(scriptFail.Code) != "" {
		failureCode = strings.TrimSpace(scriptFail.Code)
	}
	b.finishDispatch(reservation, task, func(ctx context.Context) error {
		return b.dispatchReporter().PublishFailed(
			ctx,
			reservation.ref,
			failureCode,
			err.Error(),
			failureDetail,
		)
	})
}

func (b *legionJobBridge) finishDispatch(
	reservation *dispatchReservation,
	task *Task,
	publish func(context.Context) error,
) {
	_, release := b.admissions().TaskAndRelease(reservation)
	if release != nil {
		release()
	}
	if b.admissions().MarkTerminal(reservation) {
		if err := retryDispatchEvent(publish); err != nil {
			logDispatchPublishError("terminal", err)
		}
	}
	if task != nil {
		b.agent.manager.RemoveAttempt(task.AttemptID)
	}
	b.admissions().CompactTerminal(reservation)
}

func retryDispatchEvent(publish func(context.Context) error) error {
	if publish == nil {
		return nil
	}
	var err error
	for attempt := 0; attempt < dispatchEventRetryAttempts; attempt++ {
		if err = publish(context.Background()); err == nil {
			return nil
		}
		if attempt+1 < dispatchEventRetryAttempts && dispatchEventRetryDelay > 0 {
			time.Sleep(dispatchEventRetryDelay)
		}
	}
	return err
}

func (b *legionJobBridge) handleCancel(raw []byte) error {
	return b.handleCancelForSession("", raw)
}

func (b *legionJobBridge) handleCancelForSession(sessionID string, raw []byte) error {
	var command jobv1.CancelJobCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal cancel command: %w", err)
	}
	subtaskID := command.GetJob().GetSubtaskId()
	if strings.TrimSpace(subtaskID) == "" {
		return fmt.Errorf("cancel command subtask_id is required")
	}

	reason := strings.TrimSpace(command.GetReason())
	if reason == "" {
		reason = "platform cancel requested"
	}
	jobRef := command.GetJob()
	ref := jobExecutionRef{
		CommandID: command.GetMetadata().GetCommandId(),
		JobID:     jobRef.GetJobId(),
		SubtaskID: jobRef.GetSubtaskId(),
		AttemptID: jobRef.GetAttemptId(),
	}
	beforeStart, running, matched, staleSession := b.admissions().CancelJob(sessionID, ref)
	if staleSession {
		return nil
	}
	for _, reservation := range beforeStart {
		b.cancelBeforeStart(reservation, reason)
	}
	for _, reservation := range running {
		b.cancelRunning(reservation, reason)
	}
	if matched || len(beforeStart)+len(running) > 0 {
		return nil
	}
	// Consumer-delivered cancellation is session-scoped. Every Legion dispatch
	// is registered before entering TaskManager, so falling back to the global
	// task indexes here would reopen a TOCTOU window: the old consumer could
	// pass its registry fence, a session switch could register a retry, and the
	// old cancel could then kill the new task. Preserve TaskManager fallback only
	// for the direct legacy adapter, which has no session identity.
	if sessionID != "" {
		if b.admissions().RecordPendingCancel(sessionID, ref, reason) {
			return nil
		}
		logCancelTargetMissing(subtaskID)
		return nil
	}

	var tasks []*Task
	if strings.TrimSpace(ref.AttemptID) != "" {
		if task, err := b.agent.manager.GetTaskByAttemptID(ref.AttemptID); err == nil &&
			task.JobID == ref.JobID && task.SubtaskID == ref.SubtaskID {
			tasks = append(tasks, task)
		}
	} else {
		tasks = b.agent.manager.TasksBySubtask(subtaskID)
	}
	if len(tasks) == 0 && strings.TrimSpace(ref.AttemptID) == "" {
		if task, err := b.agent.manager.GetTaskById(taskIDForSubtask(subtaskID)); err == nil {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 {
		if b.admissions().RecordPendingCancel("", ref, reason) {
			return nil
		}
		logCancelTargetMissing(subtaskID)
		return nil
	}
	for _, task := range tasks {
		task.SetCancelReason(reason)
		task.MarkCancelRequested()
		task.Cancel()
	}
	return nil
}

func (b *legionJobBridge) cancelBeforeStart(reservation *dispatchReservation, reason string) {
	task, release := b.admissions().TaskAndRelease(reservation)
	if release != nil {
		release()
	}
	if task != nil {
		task.SetCancelReason(reason)
		task.MarkCancelRequested()
		task.Cancel()
	}
	go func() {
		if err := retryDispatchEvent(func(ctx context.Context) error {
			return b.dispatchReporter().PublishCancelled(ctx, reservation.ref, reason)
		}); err != nil {
			logDispatchPublishError("cancelled", err)
		}
		if task != nil {
			b.agent.manager.RemoveAttempt(task.AttemptID)
		}
		b.admissions().CompactTerminal(reservation)
	}()
}

func (b *legionJobBridge) cancelRunning(reservation *dispatchReservation, reason string) {
	task, _ := b.admissions().TaskAndRelease(reservation)
	if task == nil {
		return
	}
	task.SetCancelReason(reason)
	task.MarkCancelRequested()
	task.Cancel()
}

func dispatchCommandIdentity(command *jobv1.DispatchJobCommand) (string, error) {
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(command)
	if err != nil {
		return "", fmt.Errorf("marshal dispatch identity: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func dispatchAdmissionDeadline(
	command *jobv1.DispatchJobCommand,
	heartbeatInterval time.Duration,
	now time.Time,
) time.Time {
	if heartbeatInterval <= 0 {
		heartbeatInterval = time.Second
	}
	issuedAt := command.GetMetadata().GetIssuedAt()
	if issuedAt != nil && issuedAt.IsValid() {
		return issuedAt.AsTime().UTC().Add(heartbeatInterval)
	}
	return now.Add(heartbeatInterval)
}

func (b *legionJobBridge) heartbeatInterval() time.Duration {
	if b.agent != nil && b.agent.heartbeatInterval > 0 {
		return b.agent.heartbeatInterval
	}
	return time.Second
}

func (b *legionJobBridge) switchDispatchSession(sessionID string) {
	for _, reservation := range b.admissions().SwitchSession(sessionID) {
		state, _, _, _ := b.admissions().Snapshot(reservation)
		task, release := b.admissions().TaskAndRelease(reservation)
		if task != nil {
			task.SetCancelReason("node session replaced")
			task.MarkCancelRequested()
			task.Cancel()
		}
		if state != dispatchAdmissionRunning {
			if release != nil {
				release()
			}
			if task != nil {
				b.agent.manager.RemoveAttempt(task.AttemptID)
			}
		}
	}
}

func (b *legionJobBridge) BeginShutdown() {
	if b == nil || !b.shuttingDown.CompareAndSwap(false, true) {
		return
	}
	b.stopConsumer()
	beforeStart, running := b.admissions().BeginShutdown()
	for _, reservation := range beforeStart {
		b.cancelBeforeStart(reservation, "node shutdown")
	}
	for _, reservation := range running {
		b.cancelRunning(reservation, "node shutdown")
	}
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
	return b.dispatchReporter().PublishFailed(ctx, ref, code, message, dispatchFailureDetail(command))
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
