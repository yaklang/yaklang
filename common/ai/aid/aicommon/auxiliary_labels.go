package aicommon

// AuxiliaryCallerLabel constants identify auxiliary AI tasks.
// These values identify tasks whose execution path explicitly selects the
// Speed model tier (LiteForge or direct CallSpeedPriorityAI),
// which are formatted as liteforge[<actionName>] at aiforge/liteforge.go
// and written into AIRequest.CallerLabel.
const (
	// ── Skip (subsystem entry gates + non-critical auxiliary steps) ──
	CallerLabelSessionInitGenerator       = "session-init-generator"
	CallerLabelSessionTitleGenerator      = "session-title-generator"
	CallerLabelIntentKeywordGen           = "intent-keyword-gen"
	CallerLabelIntentCapabilityRecommend  = "intent-capability-recommend"
	CallerLabelToolCallReason             = "tool-call-reason"
	CallerLabelToolCallIntervalReview     = "toolcall-interval-review"
	CallerLabelAIValueFeedback            = "ai_value_feedback"
	CallerLabelI18nTranslation            = "i18n-translation"
	CallerLabelTaskShortID                = "task-short-id"
	CallerLabelSmartEvaluation            = "smart-evaluation"
	CallerLabelInsufficientReasonAnalysis = "insufficient-reason-analysis"
	// Subsystem defensive fallback (should not be reached after entry gate)
	CallerLabelMemoryTriage             = "memory-triage"
	CallerLabelTextagSelection          = "tag-selection"
	CallerLabelBatchMemoryDeduplication = "batch-memory-deduplication"
	CallerLabelPerception               = "perception"

	// ── LiteCall ──
	CallerLabelExtractExploreTargetPath    = "extract-explore-target-path"
	CallerLabelExtractHTTPRequestFromInput = "extract-http-request-from-user-input"
	CallerLabelAnalyzeReportIntent         = "analyze-report-intent"
	CallerLabelCapabilityCatalogMatch      = "capability-catalog-match"
	CallerLabelKnowledgeCompress           = "knowledge-compress"
	CallerLabelKnowledgeCompressBench      = "knowledge-compress-bench"
	CallerLabelSelectKnowledgeBase         = "select_knowledge_base"
	CallerLabelEvaluateNextSearch          = "evaluate-next-search"
	CallerLabelPlanFactsHook               = "plan_facts_hook"
	CallerLabelPlanDirect                  = "plan_direct"
	CallerLabelAnalyzeRequirementAndSearch = "analyze-requirement-and-search"
	CallerLabelExtractRankedLines          = "extract-ranked-lines"
	CallerLabelHttpFuzztestInitBootstrap   = "http_fuzztest_init_booststrap"
	CallerLabelScanPlan                    = "scan_plan"
	CallerLabelLLMRerank                   = "llm-rerank"
	CallerLabelTimelineBatchCompress       = "timeline-batch-compress"
	CallerLabelTimelineHeadRefine          = "timeline-head-refine"
	CallerLabelCrawlerJSPathExtract        = "crawler-js-path-extract"

	// ── PassThrough (explicitly registered for audit) ──
	CallerLabelHttpFlowAnalyzeFinalizeSummary = "http_flow_analyze_finalize_summary"
	CallerLabelSkillConflictResolver          = "skill-conflict-resolver"
)
