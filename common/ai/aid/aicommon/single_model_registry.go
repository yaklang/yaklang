package aicommon

import "sync"

// SingleModelAction is the decision for an auxiliary task in single-model mode.
type SingleModelAction int

const (
	// SingleModelSkip means the auxiliary task is not called at all.
	// OnResult is not invoked, so downstream logic naturally does not execute.
	SingleModelSkip SingleModelAction = iota
	// SingleModelLiteCall means the task is called on the same model but with
	// degraded parameters (e.g. thinking=none) to reduce cost/latency.
	SingleModelLiteCall
	// SingleModelPassThrough means the task is called normally, no degradation.
	SingleModelPassThrough
)

// singleModelRegistry holds the action decision for each auxiliary CallerLabel.
var (
	singleModelRegistry     = map[string]SingleModelAction{}
	singleModelRegistryOnce sync.Once
)

// initSingleModelRegistry populates the registry with all known auxiliary
// CallerLabels and their default single-model-mode actions.
func initSingleModelRegistry() {
	register := func(name string, action SingleModelAction) {
		singleModelRegistry[name] = action
	}

	// ===== Skip (non-critical auxiliary: skip with fallback) =====
	// Pure display / evaluation — skipping only loses cosmetic output.
	register(CallerLabelToolCallReason, SingleModelSkip)
	register(CallerLabelSmartEvaluation, SingleModelSkip)
	register(CallerLabelInsufficientReasonAnalysis, SingleModelSkip)

	// Subsystem defensive fallback (should not be reached after entry gate)
	register(CallerLabelSessionInitGenerator, SingleModelSkip)
	register(CallerLabelSessionTitleGenerator, SingleModelSkip)
	register(CallerLabelIntentKeywordGen, SingleModelSkip)
	register(CallerLabelIntentCapabilityRecommend, SingleModelSkip)
	register(CallerLabelMemoryTriage, SingleModelSkip)
	register(CallerLabelTextagSelection, SingleModelSkip)
	register(CallerLabelBatchMemoryDeduplication, SingleModelSkip)
	register(CallerLabelPerception, SingleModelSkip)

	// ===== LiteCall (critical path: still call, but degraded) =====
	// InitTask key steps — skipping would break loop startup.
	register(CallerLabelExtractExploreTargetPath, SingleModelLiteCall)
	register(CallerLabelExtractHTTPRequestFromInput, SingleModelLiteCall)
	register(CallerLabelAnalyzeReportIntent, SingleModelLiteCall)
	register(CallerLabelKnowledgeCompress, SingleModelLiteCall)
	register(CallerLabelKnowledgeCompressBench, SingleModelLiteCall)
	register(CallerLabelSelectKnowledgeBase, SingleModelLiteCall)
	register(CallerLabelEvaluateNextSearch, SingleModelLiteCall)
	register(CallerLabelEvaluateInternetResearchNext, SingleModelLiteCall)
	register(CallerLabelPlanFactsHook, SingleModelLiteCall)
	register(CallerLabelPlanDirect, SingleModelLiteCall)
	register(CallerLabelCapabilityCatalogMatch, SingleModelLiteCall)
	register(CallerLabelAnalyzeRequirementAndSearch, SingleModelLiteCall)
	register(CallerLabelExtractRankedLines, SingleModelLiteCall)
	register(CallerLabelHttpFuzztestInitBootstrap, SingleModelLiteCall)
	register(CallerLabelScanPlan, SingleModelLiteCall)
	register(CallerLabelSubReactAgentGoalElaboration, SingleModelLiteCall)
	register(CallerLabelLLMRerank, SingleModelLiteCall)
	register(CallerLabelTimelineBatchCompress, SingleModelLiteCall)
	register(CallerLabelTimelineHeadRefine, SingleModelLiteCall)
	register(CallerLabelCrawlerJSPathExtract, SingleModelLiteCall)

	// ===== PassThrough (explicitly registered for audit) =====
	register(CallerLabelHttpFlowAnalyzeFinalizeSummary, SingleModelPassThrough)
	register(CallerLabelPlanFromDocument, SingleModelPassThrough)
	register(CallerLabelPlanGuidanceDocument, SingleModelPassThrough)
	register(CallerLabelSkillConflictResolver, SingleModelPassThrough)
}

// ensureSingleModelRegistry initializes the registry once.
func ensureSingleModelRegistry() {
	singleModelRegistryOnce.Do(initSingleModelRegistry)
}

// GetSingleModelAction returns the single-model-mode action for the given
// CallerLabel. If the label is not in the registry, the default is PassThrough
// (i.e. unregistered tasks are called normally, matching current behavior).
// Config.ScheduleAuxiliaryTask consults this registry only when single-model
// mode is enabled.
func GetSingleModelAction(name string) SingleModelAction {
	ensureSingleModelRegistry()
	if action, ok := singleModelRegistry[name]; ok {
		return action
	}
	return SingleModelPassThrough
}
