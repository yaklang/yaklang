package reactloops

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// ShouldTriggerPeriodicCheckpointOnIteration reports whether periodic
// checkpoints such as perception/verification should run on this iteration.
func (r *ReActLoop) ShouldTriggerPeriodicCheckpointOnIteration(iterationIndex int) bool {
	if r == nil {
		return false
	}
	interval := r.periodicCheckpointInterval
	if interval <= 0 {
		interval = perceptionDefaultIterationInterval
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
	result, err := r.invoker.VerifyUserSatisfaction(ctx, originalQuery, isToolCall, payload)
	if err != nil {
		return nil, err
	}
	r.ApplyVerificationResult(result)
	return result, nil
}

// MaybeVerifyUserSatisfaction gates generic automatic verification to avoid
// running it after every tool call.
func (r *ReActLoop) MaybeVerifyUserSatisfaction(
	ctx context.Context,
	originalQuery string,
	isToolCall bool,
	payload string,
) (*aicommon.VerifySatisfactionResult, bool, error) {
	if r == nil || r.invoker == nil {
		return nil, false, nil
	}
	if !r.ShouldTriggerPeriodicCheckpointOnIteration(r.GetCurrentIterationIndex()) {
		return nil, false, nil
	}
	result, err := r.invoker.VerifyUserSatisfaction(ctx, originalQuery, isToolCall, payload)
	if err != nil {
		return nil, true, err
	}
	r.ApplyVerificationResult(result)
	return result, true, nil
}
