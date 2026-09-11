package sessionruntime

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// FixOptionsWithServiceName preserves the legacy AIService selection behavior
// for every ReAct caller, rather than leaving it in the gRPC adapter.
func FixOptionsWithServiceName(serviceName string, opts ...aicommon.ConfigOption) []aicommon.ConfigOption {
	aiCb, err := aicommon.CreateCallbackFromConfig(aiconfig.GetGlobalManager().GetFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, serviceName, ""))
	if err != nil {
		log.Errorf("load ai service failed: %v", err)
	} else {
		opts = append(opts, aicommon.WithAutoTieredAICallback(aiCb))
	}
	log.Warnf("AIStartParams.AIService/AIModelName for WithAIChatInfo is deprecated, " +
		"model info is now auto-detected from the actual AI gateway call")
	return opts
}

// ConvertStartParamsToReActConfig converts the shared protobuf input model to
// ReAct config options. The protobuf model is retained because ReAct itself
// still consumes it; this package is independent of the gRPC stream transport.
func ConvertStartParamsToReActConfig(i *ypb.AIStartParams) []aicommon.ConfigOption {
	opts := make([]aicommon.ConfigOption, 0)
	if i == nil {
		return opts
	}
	enableMultiAgent, goalModeEnabled, goalMinIterations, maxSubAgents := resolveAIExecutionStrategy(i)
	if i.DisallowRequireForUserPrompt {
		opts = append(opts, aicommon.WithAllowRequireForUserInteract(false))
	} else {
		opts = append(opts, aicommon.WithAllowRequireForUserInteract(true))
	}

	if i.ReviewPolicy != "" {
		opts = append(opts, aicommon.WithAgreePolicy(aicommon.AgreePolicyType(i.ReviewPolicy)))
	}

	if i.GetAIReviewRiskControlScore() > 0 {
		opts = append(opts, aicommon.WithAgreeAIRiskCtrlScore(i.GetAIReviewRiskControlScore()))
	}

	if i.ReActMaxIteration > 0 {
		maxIterations := i.GetReActMaxIteration()
		// Goal-mode entry-point normalization: raise a too-small ceiling so the
		// finish gate (GoalMinIterations) can open before iterations run out.
		// loop_default.resolveMaxIterations applies the same bump idempotently
		// as a safety net for programmatic (non-gRPC) entry paths.
		if goalModeEnabled {
			maxIterations = aicommon.EnsureGoalModeMaxIterations(maxIterations, goalMinIterations)
		}
		opts = append(opts, aicommon.WithMaxIterationCount(maxIterations))
	}

	if i.GetTimelineContentSizeLimit() > 0 {
		opts = append(opts, aicommon.WithTimelineContentLimit(int(i.GetTimelineContentSizeLimit())))
	}

	if i.UserInteractLimit > 0 {
		opts = append(opts, aicommon.WithPlanUserInteractMaxCount(i.UserInteractLimit))
	}

	if i.GetDisableToolUse() {
		opts = append(opts, aicommon.WithDisableToolUse(true))
	}
	if i.GetEnableAISearchTool() {
		opts = append(opts, aid.WithAiToolsSearchTool())
	}
	if enableMultiAgent {
		opts = append(opts,
			aicommon.WithEnableMultiAgentMode(true),
			aicommon.WithMaxSubAgents(maxSubAgents),
		)
	}
	if len(i.GetExcludeToolNames()) > 0 {
		opts = append(opts, aicommon.WithDisableToolsName(i.GetExcludeToolNames()...))
	}
	if len(i.GetIncludeSuggestedToolNames()) > 0 {
		opts = append(opts, aicommon.WithEnableToolsName(i.GetIncludeSuggestedToolNames()...))
	}
	if len(i.GetIncludeSuggestedToolKeywords()) > 0 {
		opts = append(opts, aicommon.WithKeywords(i.GetIncludeSuggestedToolKeywords()...))
	}
	if i.GetAIService() != "" {
		opts = FixOptionsWithServiceName(i.GetAIService(), opts...)
	}

	if !i.GetDisableAISearchForge() {
		opts = append(opts, aid.WithAiForgeSearchTool())
	}

	// EnablePlan 晚于 DisableAISearchForge 应用，用于控制 PE / 蓝图动作；与 AI 搜索 Forge 工具独立。
	opts = append(opts, aicommon.WithEnablePlanAndExec(i.GetEnablePlan()))
	if i.GetEnableDetachedPlan() {
		opts = append(opts, aicommon.WithEnableDetachedPlan(true))
	}

	if i.GetAICallTokenLimit() > 0 {
		opts = append(opts, aicommon.WithAiCallTokenLimit(int64(i.GetAICallTokenLimit())))
	}

	if i.GetDisableToolIntervalReview() {
		opts = append(opts, aicommon.WithDisableToolCallerIntervalReview(true))
	}
	if i.GetSyncPerceptionTrigger() {
		opts = append(opts, aicommon.WithSyncPerceptionTrigger(true))
	}
	if goalModeEnabled {
		opts = append(opts,
			aicommon.WithEnableGoalMode(true),
			aicommon.WithGoalMinIterations(goalMinIterations),
		)
	}

	if i.GetUserPresetPrompt() != "" {
		opts = append(opts, aicommon.WithUserPresetPrompt(i.GetUserPresetPrompt()))
	}

	if i.GetPlanExecTaskConcurrency() > 0 {
		opts = append(opts, aicommon.WithPlanExecTaskConcurrency(int(i.GetPlanExecTaskConcurrency())))
	}

	if i.GetUserPlanPrompt() != "" {
		opts = append(opts, aicommon.WithPlanPrompt(i.GetUserPlanPrompt()))
	}

	if i.GetSource() != "" {
		opts = append(opts, aicommon.WithSessionSource(i.GetSource()))
	}

	if caps := aicommon.ParseEnabledCapabilitiesFromProto(i); len(caps) > 0 {
		opts = append(opts, aicommon.WithEnabledCapabilities(caps...))
	}

	if i.GetDisableMemoryTriage() {
		opts = append(opts, aicommon.WithDisableMemoryTriage(true))
	}

	return opts
}

func resolveAIExecutionStrategy(i *ypb.AIStartParams) (enableMultiAgent bool, enableGoalMode bool, goalMinIterations int64, maxSubAgents int64) {
	if i == nil {
		return false, false, aicommon.DefaultGoalMinIterations, aicommon.DefaultMaxSubAgentConcurrency
	}
	if strategy := i.GetStrategy(); strategy != nil {
		enableMultiAgent = strategy.GetEnableMultiAgent()
		enableGoalMode = strategy.GetEnableGoalMode()
		goalMinIterations = strategy.GetGoalMinIterations()
		maxSubAgents = strategy.GetMaxSubAgents()
	}
	goalMinIterations = aicommon.NormalizeGoalMinIterations(goalMinIterations)
	return
}

// ResolveSessionStartParams optionally overlays request parameters on the
// durable session config. Keeping this in the runtime package gives every
// transport and background caller identical startup semantics.
func ResolveSessionStartParams(db *gorm.DB, sessionID string, request *ypb.AIStartParams, preferCached bool) (*ypb.AIStartParams, error) {
	if request == nil {
		request = &ypb.AIStartParams{}
	}
	if !preferCached || db == nil || strings.TrimSpace(sessionID) == "" {
		return request, nil
	}

	if _, err := yakit.GetAISessionMetaBySessionID(db, sessionID); err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return request, nil
		}
		return nil, err
	}

	cached, err := yakit.GetAISessionMetaStartParamsBySessionID(db, sessionID)
	if err != nil {
		return nil, err
	}
	if cached == nil {
		return request, nil
	}
	return yakit.MergeCachedAISessionStartParams(cached, request), nil
}
