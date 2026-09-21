package aicommon

import "sync"

// SingleModelAction is the decision for an auxiliary task in single-model mode.
type SingleModelAction int

const (
	// SingleModelSkip means the auxiliary task is not called at all.
	// OnResult is not invoked, so downstream logic naturally does not execute.
	SingleModelSkip SingleModelAction = iota
	// SingleModelRun executes the already-bound Speed callback.
	SingleModelRun
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
	register(CallerLabelToolCallIntervalReview, SingleModelSkip)
	register(CallerLabelAIValueFeedback, SingleModelSkip)
	register(CallerLabelI18nTranslation, SingleModelSkip)
	register(CallerLabelTaskShortID, SingleModelSkip)
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

}

// ensureSingleModelRegistry initializes the registry once.
func ensureSingleModelRegistry() {
	singleModelRegistryOnce.Do(initSingleModelRegistry)
}

// GetSingleModelAction returns the single-model-mode action for the given
// CallerLabel. If the label is not in the registry, the default is Run
// All non-skipped tasks use Speed; single-model Speed was bound to LiteCall at initialization.
// Config.ResolveAuxiliaryTask consults this registry only when single-model
// mode is enabled; this registry does not control request parameters.
func GetSingleModelAction(name string) SingleModelAction {
	ensureSingleModelRegistry()
	if action, ok := singleModelRegistry[name]; ok {
		return action
	}
	return SingleModelRun
}
