package aicommon

// SubForgeExecutionPolicy copies the caller's model and execution policy
// without its prompts, timeline, tools, user input or preset plan.
func SubForgeExecutionPolicy(parent *Config) []ConfigOption {
	return []ConfigOption{
		WithAICallbacks(parent.GetRawAICallbacks()),
		WithAIChatInfo(parent.AiServerName, parent.AiModelName),
		WithDisablePerception(parent.DisablePerception),
		WithNoOpMemoryTriage(),
		WithEnableFunctionCallMode(parent.EnableFunctionCallMode),
		WithAgreePolicy(parent.AgreePolicy),
		WithAgreeAIScoreLowMid(parent.AgreeAIScoreLow, parent.AgreeAIScoreMiddle),
		WithAgreeInterval(parent.AgreeInterval),
		WithAgreeManualCallback(parent.AgreeManualCallback),
		WithForceManualPlanReview(parent.ForceManualPlanReview),
		WithAiPlanReviewControl(parent.AiPlanReviewControl),
		WithAiTaskReviewControl(parent.AiTaskReviewControl),
		WithAllowRequireForUserInteract(parent.AllowRequireForUserInteract),
		WithAllowPlanUserInteract(parent.AllowPlanUserInteract),
		WithPlanUserInteractMaxCount(parent.PlanUserInteractMaxCount),
		WithPerTaskUserInteractiveLimitedTimes(parent.PerTaskUserInteractiveLimitedTimes),
		WithMaxIterationCount(parent.GetMaxIterationCount()),
		WithDisableIncreaseIteration(parent.DisableIncreaseIteration),
		WithAiCallTokenLimit(parent.AiCallTokenLimit),
		WithAITransactionRetry(parent.AiTransactionAutoRetry),
		WithAIAutoRetry(parent.AiAutoRetry),
		WithPlanExecTaskConcurrency(parent.GetPlanExecTaskConcurrency()),
		WithToolComposeConcurrency(parent.GetToolComposeConcurrency()),
		WithToolBatchMaxCalls(parent.GetConfigInt(ConfigKeyToolBatchMaxCalls, DefaultToolBatchMaxCalls)),
		WithToolBatchInvokeConcurrency(parent.GetConfigInt(ConfigKeyToolBatchInvokeConcurrency, DefaultToolBatchInvokeConcurrency)),
		WithMaxTaskContinue(parent.MaxTaskContinue),
		WithTimelineContentLimit(parent.TimelineContentSizeLimit),
		WithAIRetryWaitFunc(parent.aiRetryWaitFunc),
		WithUserUsageCallback(parent.GetUserUsageCallback()),
		WithReActActionPolicy(parent.reActActionPolicy),
		func(child *Config) error {
			child.DisableToolUse = child.DisableToolUse || parent.DisableToolUse
			child.disableBlockingClarification = parent.disableBlockingClarification || parent.EnableGoalMode
			return nil
		},
	}
}
