// Package syntaxflow_utils holds shared domain logic for IRify / SyntaxFlow / SSA Risk ReAct loops:
// attachment wire constants, input resolution from loop vars and AttachedResource payloads, scan-session
// DB helpers, optional shared LoopAction factories, and reflection-output appendix text.
//
// # Attachment contract (Yakit / wire strings are stable)
//
// Go name in this package	Wire Type / Key / Value
//
// AttachedTypeSyntaxFlow	"irify_syntaxflow"
//
// AttachedTypeSSARisk	"irify_ssa_risk"
//
// AttachedTypeSSARisksFilter	"irify_ssa_risks_filter"
//
// AttachedTypeSyntaxFlowRule	"irify_syntaxflow_rule"
//
// AttachedKeyTaskID … FullQuality	keys: task_id, session_mode, programs, risk_id, filter_json, runtime_id, program_name, full_quality
//
// SessionModeAttach / SessionModeStart	values: attach, start
//
// # JSON Schema vs reflection OutputExample
//
// Per-iteration Prompt Schema comes from reactloops.buildSchema: it flattens each registered LoopAction’s
// aitool.ToolOption into one object (alongside @action, identifier, human_readable_thought).
// Each loop that needs these fields registers the shared options once in its factory preset.
//
// WithReflectionOutputExample renders the loop’s embedded reflection_output_example.txt, then may append
// OutputExamples from globally registered LoopAction metadata. Actions registered only via WithRegisterLoopAction
// on the loop instance are often not in that global map; therefore shared few-shots live in
// ReflectionOutputSharedAppendix and should be concatenated in the loop’s WithReflectionOutputExample.
//
// # Shared LoopAction factories
//
//   - WithReloadSSARiskOverviewAction — reload_ssa_risk_overview
//   - WithReloadSyntaxFlowScanSessionAction — reload_syntaxflow_scan_session
//   - WithSetSSARiskReviewTargetAction — set_ssa_risk_review_target
//
// Parameters and semantics are documented on each With* function and in aitool.WithParam_Description strings.
package syntaxflow_utils
