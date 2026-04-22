package reactloops

import (
	"context"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const (
	verificationAutoTriggerMaxSnapshotAge = 30 * time.Second
	verificationAutoTriggerMinPromptDelta = 500
	verificationIterationTriggerInterval  = 5
)

var verificationWatchdogIdleTimeout = 2 * time.Minute

type VerificationRuntimeSnapshot struct {
	GeneratedAt      time.Time `json:"generated_at"`
	IterationIndex   int       `json:"iteration_index"`
	LoopPromptTokens int       `json:"loop_prompt_tokens,omitempty"`
}

// RefreshVerificationRuntimeSnapshot rebuilds the runtime snapshot from the
// current loop state and replaces the previously stored snapshot pointer.
func (r *ReActLoop) RefreshVerificationRuntimeSnapshot() *VerificationRuntimeSnapshot {
	if r == nil {
		return nil
	}

	snapshot := r.buildVerificationRuntimeSnapshot(time.Now())
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()
	r.verificationRuntimeSnapshot = snapshot
	return r.verificationRuntimeSnapshot
}

// GetVerificationRuntimeSnapshot returns the currently stored snapshot pointer.
// Callers should treat the returned pointer as read-only.
func (r *ReActLoop) GetVerificationRuntimeSnapshot() *VerificationRuntimeSnapshot {
	if r == nil {
		return nil
	}

	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()
	return r.verificationRuntimeSnapshot
}

func (r *ReActLoop) buildVerificationRuntimeSnapshot(generatedAt time.Time) *VerificationRuntimeSnapshot {
	if r == nil {
		return nil
	}

	snapshot := &VerificationRuntimeSnapshot{
		GeneratedAt:      generatedAt,
		LoopPromptTokens: r.getVerificationRuntimePromptTokens(),
	}
	r.actionHistoryMutex.Lock()
	snapshot.IterationIndex = r.currentIterationIndex
	r.actionHistoryMutex.Unlock()
	return snapshot
}

func (r *ReActLoop) setVerificationRuntimeSnapshot(snapshot *VerificationRuntimeSnapshot) {
	if r == nil {
		return
	}
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()
	r.verificationRuntimeSnapshot = snapshot
}

func (r *ReActLoop) getVerificationRuntimePromptTokens() int {
	if r == nil {
		return 0
	}
	if observation := r.GetLastPromptObservation(); observation != nil {
		return observation.PromptTokens
	}
	if status := r.GetLastPromptObservationStatus(); status != nil {
		return status.PromptTokens
	}
	return 0
}

// ShouldTriggerPeriodicCheckpointOnIteration reports whether periodic
// checkpoints such as perception/verification should run on this iteration.
func (r *ReActLoop) ShouldTriggerPeriodicCheckpointOnIteration(iterationIndex int) bool {
	if r == nil {
		return false
	}
	interval := r.periodicVerificationCount
	if interval <= 0 {
		return true
	}
	if iterationIndex > 0 && iterationIndex%interval == 0 {
		return true
	}
	return r.maxIterations > 0 && iterationIndex > 0 && iterationIndex == r.maxIterations
}

// ApplyVerificationResult stores verification side effects in the loop state.
func (r *ReActLoop) ApplyVerificationResult(result *aicommon.VerifySatisfactionResult) {
	if r == nil || result == nil {
		return
	}

	cfg := r.GetConfig()
	if cfg != nil && len(result.OutputFiles) > 0 {
		if providerManager := cfg.GetContextProviderManager(); providerManager != nil {
			for _, filePath := range result.OutputFiles {
				providerName := "output_file:" + filePath
				providerManager.RegisterTracedContent(
					providerName,
					aicommon.OutputFileContextProvider(filePath),
				)
				if emitter := cfg.GetEmitter(); emitter != nil {
					emitter.EmitPinFilename(filePath)
				}
			}
		}
	}

	r.PushSatisfactionRecordWithCompletedTaskIndex(
		result.Satisfied,
		result.Reasoning,
		result.CompletedTaskIndex,
		result.NextMovements,
		result.Evidence,
		result.OutputFiles,
		result.EvidenceOps,
	)
	if cfg != nil && len(result.EvidenceOps) > 0 {
		cfg.ApplySessionEvidenceOps(result.EvidenceOps)
	}
	r.MaybeTriggerPerceptionAfterVerification()
}

// VerifyUserSatisfactionNow forces a verification pass immediately, bypassing
// periodic checkpoint throttling. This is used by explicit AI-triggered
// verification actions.
func (r *ReActLoop) VerifyUserSatisfactionNow(
	ctx context.Context,
	originalQuery string,
	isToolCall bool,
	payload string,
) (*aicommon.VerifySatisfactionResult, error) {
	if r == nil || r.invoker == nil {
		return nil, nil
	}
	r.touchVerificationWatchdog()
	if r.verificationMutex != nil {
		r.verificationMutex.Lock()
		defer r.verificationMutex.Unlock()
	}
	result, err := r.invoker.VerifyUserSatisfaction(ctx, originalQuery, isToolCall, payload)
	if err != nil {
		return nil, err
	}
	r.setVerificationRuntimeSnapshot(r.buildVerificationRuntimeSnapshot(time.Now()))
	r.ApplyVerificationResult(result)
	return result, nil
}

// MaybeVerifyUserSatisfaction gates generic automatic verification to avoid
// running it after every tool call. In addition to the shared periodic
// checkpoint rule, it only re-runs auto-verification when the last accepted
// verification baseline is absent, stale, or the loop prompt has changed
// materially.
func (r *ReActLoop) MaybeVerifyUserSatisfaction(
	ctx context.Context,
	originalQuery string,
	isToolCall bool,
	payload string,
) (*aicommon.VerifySatisfactionResult, bool, error) {
	if r == nil || r.invoker == nil {
		return nil, false, nil
	}
	r.touchVerificationWatchdog()
	currentSnapshot := r.buildVerificationRuntimeSnapshot(time.Now())
	if !r.shouldTriggerAutomaticVerification(currentSnapshot) {
		return nil, false, nil
	}
	if r.verificationMutex != nil {
		r.verificationMutex.Lock()
		defer r.verificationMutex.Unlock()
	}
	currentSnapshot = r.buildVerificationRuntimeSnapshot(time.Now())
	if !r.shouldTriggerAutomaticVerification(currentSnapshot) {
		return nil, false, nil
	}
	result, err := r.invoker.VerifyUserSatisfaction(ctx, originalQuery, isToolCall, payload)
	if err != nil {
		return nil, true, err
	}
	r.setVerificationRuntimeSnapshot(currentSnapshot)
	r.ApplyVerificationResult(result)
	return result, true, nil
}

func (r *ReActLoop) startVerificationWatchdog(task aicommon.AIStatefulTask) {
	if r == nil || task == nil || r.verificationMutex == nil {
		return
	}
	r.verificationMutex.Lock()
	defer r.verificationMutex.Unlock()
	if r.verificationWatchdogTimer != nil {
		r.verificationWatchdogTimer.Stop()
	}
	r.verificationWatchdogTimer = time.AfterFunc(verificationWatchdogIdleTimeout, func() {
		r.triggerVerificationWatchdog(task)
	})
}

func (r *ReActLoop) touchVerificationWatchdog() {
	if r == nil {
		return
	}
	task := r.GetCurrentTask()
	if task == nil {
		return
	}
	r.startVerificationWatchdog(task)
}

func (r *ReActLoop) stopVerificationWatchdogForTask(task aicommon.AIStatefulTask) {
	if r == nil || task == nil || r.verificationMutex == nil {
		return
	}
	r.verificationMutex.Lock()
	defer r.verificationMutex.Unlock()
	if r.verificationWatchdogTimer != nil {
		r.verificationWatchdogTimer.Stop()
		r.verificationWatchdogTimer = nil
	}
}

func (r *ReActLoop) triggerVerificationWatchdog(task aicommon.AIStatefulTask) {
	if r == nil || task == nil || task.IsFinished() {
		return
	}
	select {
	case <-task.GetContext().Done():
		return
	default:
	}
	if r.GetInvoker() == nil {
		return
	}
	payload := r.buildVerificationWatchdogPayload(task)
	r.GetInvoker().AddToTimeline("[ASYNC_VERIFICATION_WATCHDOG]", payload)
	result, err := r.VerifyUserSatisfactionNow(task.GetContext(), task.GetUserInput(), false, payload)
	if err != nil {
		r.GetInvoker().AddToTimeline("verification_watchdog_error", err.Error())
		return
	}
	if result == nil {
		return
	}
	if result.Satisfied {
		task.Finish(nil)
		r.stopVerificationWatchdogForTask(task)
		return
	}
	r.GetInvoker().AddToTimeline("verification_watchdog_unsatisfied", result.Reasoning)
}

func (r *ReActLoop) buildVerificationWatchdogPayload(task aicommon.AIStatefulTask) string {
	if r == nil {
		return "Verification watchdog triggered because MaybeVerifyUserSatisfaction has been idle for too long."
	}
	payload := "Verification watchdog triggered because MaybeVerifyUserSatisfaction has been idle for too long."
	payload += fmt.Sprintf("\nCurrent iteration: %d.", r.GetCurrentIterationIndex())
	if last := r.GetLastAction(); last != nil {
		payload += "\nLast action: " + last.ActionType + "."
	}
	if task != nil {
		payload += "\nPlease verify whether the current work already satisfies the user goal."
	}
	return payload
}

func (r *ReActLoop) shouldTriggerAutomaticVerification(current *VerificationRuntimeSnapshot) bool {
	if r == nil || current == nil {
		return false
	}
	if r.maxIterations > 0 && current.IterationIndex == r.maxIterations {
		return true
	}
	previous := r.GetVerificationRuntimeSnapshot()
	if previous == nil {
		return r.ShouldTriggerPeriodicCheckpointOnIteration(current.IterationIndex)
	}
	if current.GeneratedAt.Sub(previous.GeneratedAt) >= verificationAutoTriggerMaxSnapshotAge {
		return true
	}
	if current.IterationIndex-previous.IterationIndex >= r.getVerificationIterationTriggerInterval() {
		return true
	}
	return verificationPromptTokenDelta(previous, current) >= verificationAutoTriggerMinPromptDelta
}

func verificationPromptTokenDelta(previous *VerificationRuntimeSnapshot, current *VerificationRuntimeSnapshot) int {
	if previous == nil || current == nil {
		return 0
	}
	delta := current.LoopPromptTokens - previous.LoopPromptTokens
	if delta < 0 {
		return -delta
	}
	return delta
}

func (r *ReActLoop) getVerificationIterationTriggerInterval() int {
	if r == nil {
		return verificationIterationTriggerInterval
	}
	return r.periodicVerificationCount
}
