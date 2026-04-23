package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_LoadCapability = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY,
	Description: "自动加载一些外部能力，这个外部能力可以被自动检测类型，并且加载。可以出现工具调用(tool)，专注模式(focus_mode)，技能(skill)或者模版/蓝图（forge/blueprint）",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"capability_identifier",
			aitool.WithParam_Description(`只对 {"@action":"load_capability" ...} 时生效，这个标识符会被自动检测是 skill/tool/forge/focus_mode/filename, 然后自动加载`),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{
			FieldName: "capability_identifier",
			AINodeId:  "load_capability",
		},
	},
	ActionVerifier: loadCapabilityVerifier,
	ActionHandler:  loadCapabilityHandler,
}

func loadCapabilityVerifier(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	identifier := strings.TrimSpace(action.GetString("capability_identifier"))
	if identifier == "" {
		identifier = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("capability_identifier"))
	}
	if identifier == "" {
		return utils.Error("load_capability action requires 'identifier' parameter")
	}

	resolved := loop.ResolveIdentifier(identifier)
	loop.Set("_load_cap_identifier", identifier)
	loop.Set("_load_cap_resolved_type", string(resolved.IdentityType))
	loop.Set("_load_cap_suggestion", resolved.Suggestion)

	// Store alternative types for fallback in handler
	var altTypes []string
	for _, alt := range resolved.Alternatives {
		altTypes = append(altTypes, string(alt.IdentityType))
	}
	loop.Set("_load_cap_alt_types", strings.Join(altTypes, ","))

	invoker := loop.GetInvoker()

	// Track repeated attempts for the same identifier to detect loops
	attemptKey := "_load_cap_attempt_" + identifier
	prevAttempts := loop.Get(attemptKey)
	attemptCount := 1
	if prevAttempts != "" {
		fmt.Sscanf(prevAttempts, "%d", &attemptCount)
		attemptCount++
	}
	loop.Set(attemptKey, fmt.Sprintf("%d", attemptCount))

	if attemptCount > 1 {
		invoker.AddToTimeline("[LOAD_CAPABILITY_REPEATED_ATTEMPT]",
			fmt.Sprintf("WARNING: identifier='%s' has been attempted %d times. "+
				"DO NOT call load_capability with '%s' again. "+
				"You MUST try a completely different approach or identifier. "+
				"Repeating the same load_capability call is wasteful and will not produce different results.",
				identifier, attemptCount, identifier))
	}

	invoker.AddToTimeline("[LOAD_CAPABILITY_VERIFIED]",
		fmt.Sprintf("identifier='%s' resolved as '%s' (attempt #%d)", identifier, resolved.IdentityType, attemptCount))
	return nil
}

func loadCapabilityHandler(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
	identifier := loop.Get("_load_cap_identifier")
	if identifier == "" {
		op.Fail("load_capability: identifier is empty")
		return
	}

	resolvedType := aicommon.ResolvedIdentifierType(loop.Get("_load_cap_resolved_type"))
	invoker := loop.GetInvoker()

	ctx := invoker.GetConfig().GetContext()
	task := loop.GetCurrentTask()
	if task != nil {
		ctx = task.GetContext()
	}

	altTypesStr := loop.Get("_load_cap_alt_types")
	hasSkillAlt := strings.Contains(altTypesStr, string(aicommon.ResolvedAs_Skill))

	switch resolvedType {
	case aicommon.ResolvedAs_Tool:
		handleLoadTool(loop, invoker, ctx, identifier, op)
	case aicommon.ResolvedAs_Forge:
		handleLoadForgeWithSkillFallback(loop, invoker, ctx, identifier, op, hasSkillAlt)
	case aicommon.ResolvedAs_Skill:
		handleLoadSkill(loop, invoker, identifier, op)
	case aicommon.ResolvedAs_FocusedMode:
		handleLoadFocusMode(loop, invoker, ctx, identifier, op)
	default:
		handleLoadUnknown(loop, invoker, ctx, identifier, op)
	}
}

// handleLoadTool executes a tool call (mirrors require_tool logic).
func handleLoadTool(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	_ interface{ Done() <-chan struct{} },
	identifier string,
	op *reactloops.LoopActionHandlerOperator,
) {
	log.Infof("load_capability: dispatching '%s' as tool", identifier)
	invoker.AddToTimeline("[LOAD_CAPABILITY_TOOL]", fmt.Sprintf("Executing tool '%s'", identifier))

	taskCtx := invoker.GetConfig().GetContext()
	task := loop.GetCurrentTask()
	if task != nil {
		taskCtx = task.GetContext()
	}

	result, directly, err := invoker.ExecuteToolRequiredAndCall(taskCtx, identifier)
	if err != nil {
		errMsg := fmt.Sprintf("Tool '%s' execution failed: %v.", identifier, err)
		invoker.AddToTimeline("[LOAD_CAPABILITY_TOOL_ERROR]", errMsg)
		op.Feedback(errMsg + " Please try a different tool or approach.")
		op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		op.SetReflectionData("tool_error", err.Error())
		op.SetReflectionData("tool_name", identifier)
		op.Continue()
		return
	}
	if directly {
		answer, err := invoker.DirectlyAnswer(taskCtx, "在上一次工具调用中，用户中断了工具执行，要求直接回答一些问题", nil)
		if err != nil {
			op.Fail(utils.Error("DirectlyAnswer fail, reason: " + err.Error()))
			return
		}
		invoker.AddToTimeline("directly-answer", answer)
		op.Exit()
		return
	}
	if result == nil {
		invoker.AddToTimeline("error", fmt.Sprintf("load_capability tool[%v] returned nil result", identifier))
		op.Continue()
		return
	}
	if result.Error != "" {
		invoker.AddToTimeline("call["+identifier+"] error", result.Error)
	}

	task = loop.GetCurrentTask()
	if task == nil {
		op.Continue()
		return
	}
	verifyResult, err := invoker.VerifyUserSatisfaction(taskCtx, task.GetUserInput(), true, identifier)
	if err != nil {
		op.Fail(err)
		return
	}
	loop.PushSatisfactionRecordWithCompletedTaskIndex(
		verifyResult.Satisfied, verifyResult.Reasoning,
		verifyResult.CompletedTaskIndex, verifyResult.NextMovements, verifyResult.Evidence, verifyResult.OutputFiles,
		verifyResult.EvidenceOps,
	)
	if len(verifyResult.EvidenceOps) > 0 {
		loop.GetConfig().ApplySessionEvidenceOps(verifyResult.EvidenceOps)
	}
	if verifyResult.Satisfied {
		op.Exit()
		return
	}
	feedbackMsg := fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning)
	if summary := aicommon.FormatVerifyNextMovementsSummary(verifyResult.NextMovements); summary != "" {
		feedbackMsg += fmt.Sprintf("\nNext Steps: %s", summary)
	}
	op.Feedback(feedbackMsg)
	op.Continue()
}

// handleLoadForgeWithSkillFallback tries loading as forge first. If forge is rejected
// (e.g. async mode conflict) and a skill with the same name exists, falls back to skill.
func handleLoadForgeWithSkillFallback(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	ctx interface{ Done() <-chan struct{} },
	identifier string,
	op *reactloops.LoopActionHandlerOperator,
	hasSkillAlt bool,
) {
	task := loop.GetCurrentTask()
	if task != nil && task.IsAsyncMode() && hasSkillAlt {
		log.Infof("load_capability: forge '%s' rejected (async mode), falling back to skill alternative", identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_FORGE_TO_SKILL_FALLBACK]",
			fmt.Sprintf("'%s' exists as both forge and skill. Forge rejected (async conflict), falling back to skill.", identifier))
		handleLoadSkill(loop, invoker, identifier, op)
		return
	}
	handleLoadForge(loop, invoker, ctx, identifier, op)
}

// handleLoadForge starts an async blueprint/forge execution.
// If the current task is already in async mode (e.g. a PE_TASK is already running),
// the request is rejected to prevent nested async execution.
func handleLoadForge(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	ctx interface{ Done() <-chan struct{} },
	identifier string,
	op *reactloops.LoopActionHandlerOperator,
) {
	task := loop.GetCurrentTask()
	if task != nil && task.IsAsyncMode() {
		log.Warnf("load_capability: rejecting forge '%s' because current task is already in async mode", identifier)
		rejectMsg := fmt.Sprintf(
			"REJECTED: Cannot start AI Blueprint '%s' — the current task is already running in async mode. "+
				"You MUST NOT start another async operation while one is in progress. "+
				"Wait for the current async task to complete, or take a different synchronous action.",
			identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_FORGE_REJECTED]", rejectMsg)
		op.Feedback(rejectMsg)
		op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		op.SetReflectionData("forge_rejected_reason", "task_already_async")
		op.SetReflectionData("forge_name", identifier)
		op.Continue()
		return
	}

	log.Infof("load_capability: dispatching '%s' as blueprint/forge", identifier)
	invoker.AddToTimeline("[LOAD_CAPABILITY_FORGE]",
		fmt.Sprintf("Starting AI Blueprint '%s' in async mode", identifier))
	recommendCapabilitiesFromForgePrompts(loop, invoker, identifier, "AI Blueprint "+identifier)

	op.RequestAsyncMode()

	task = op.GetTask()
	task.SetAsyncMode(true)
	taskCtx := task.GetContext()
	invoker.RequireAIForgeAndAsyncExecute(taskCtx, identifier, func(err error) {
		loop.FinishAsyncTask(task, err)
	})
}

// handleLoadSkill loads a skill into the context window.
func handleLoadSkill(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	identifier string,
	op *reactloops.LoopActionHandlerOperator,
) {
	log.Infof("load_capability: dispatching '%s' as skill", identifier)

	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		invoker.AddToTimeline("[LOAD_CAPABILITY_SKILL_ERROR]", "skills context manager is not available")
		op.Feedback(fmt.Sprintf(
			"Cannot load skill '%s': skills context manager is not available. "+
				"Try using a different approach.", identifier))
		op.Continue()
		return
	}

	if mgr.IsSkillLoadedAndUnfolded(identifier) {
		viewSummary := mgr.GetSkillViewSummary(identifier)
		invoker.AddToTimeline("skill_already_loaded",
			fmt.Sprintf("Skill '%s' is already loaded", identifier))
		op.Feedback(fmt.Sprintf(
			"Skill '%s' is already loaded and active in SKILLS_CONTEXT. "+
				"Do NOT load it again. %s Proceed with your task using the loaded content.",
			identifier, viewSummary))
		op.Continue()
		return
	}

	err := mgr.LoadSkill(identifier)
	if err != nil {
		log.Warnf("load_capability: failed to load skill %q: %v", identifier, err)
		errMsg := fmt.Sprintf("Failed to load skill '%s': %v", identifier, err)
		invoker.AddToTimeline("[LOAD_CAPABILITY_SKILL_ERROR]", errMsg)

		resolved := loop.ResolveIdentifier(identifier)
		if !resolved.IsUnknown() {
			op.Feedback(errMsg + "\n\n" + resolved.Suggestion)
		} else {
			op.Feedback(errMsg + " Please verify the skill name is correct.")
		}
		op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		op.SetReflectionData("skill_load_error", err.Error())
		op.SetReflectionData("skill_name", identifier)
		op.Continue()
		return
	}

	viewSummary := mgr.GetSkillViewSummary(identifier)
	invoker.AddToTimeline("skill_loaded",
		fmt.Sprintf("Successfully loaded skill '%s' into context. %s", identifier, viewSummary))
	log.Infof("load_capability: skill %q loaded into context successfully", identifier)

	persistLoadedSkillNames(loop, invoker)
	emitSkillReferenceMaterial(invoker, identifier, mgr)
	recommendationSummary := recommendCapabilitiesFromSkillContent(loop, invoker, identifier, "Skill "+identifier)

	feedbackMsg := fmt.Sprintf(
		"Skill '%s' has been loaded into the context. "+
			"The SKILL.md content and file tree are now displayed in the SKILLS_CONTEXT section of your prompt. "+
			"Read the skill content from your prompt's View Window and proceed with the task. "+
			"Do NOT load this skill again.",
		identifier)
	if recommendationSummary != "" {
		feedbackMsg += fmt.Sprintf(" Related capabilities mentioned in SKILL.md: %s.", recommendationSummary)
	}
	op.Feedback(feedbackMsg)
	op.Continue()
}

// handleLoadFocusMode synchronously executes a focus mode loop.
// Provides detailed timeline feedback for both success and failure outcomes.
func handleLoadFocusMode(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	ctx interface{ Done() <-chan struct{} },
	identifier string,
	op *reactloops.LoopActionHandlerOperator,
) {
	log.Infof("load_capability: dispatching '%s' as focus mode loop", identifier)
	invoker.AddToTimeline("[LOAD_CAPABILITY_FOCUS_MODE]",
		fmt.Sprintf("Entering focus mode '%s' via ExecuteLoopTaskIF", identifier))

	cfg := invoker.GetConfig()
	taskCtx := cfg.GetContext()
	task := loop.GetCurrentTask()
	if task != nil {
		taskCtx = task.GetContext()
	}

	userInput := ""
	if task != nil {
		userInput = task.GetUserInput()
	}

	subTask := aicommon.NewStatefulTaskBase(
		invoker.GetCurrentTaskId()+"_focus_"+identifier,
		userInput,
		taskCtx,
		cfg.GetEmitter(),
	)

	originOptions := cfg.OriginOptions()
	var opts []any
	for _, option := range originOptions {
		opts = append(opts, option)
	}

	_, err := invoker.ExecuteLoopTaskIF(identifier, subTask, opts...)
	if err != nil {
		log.Warnf("load_capability: focus mode '%s' execution failed: %v", identifier, err)
		failMsg := fmt.Sprintf(
			"Focus mode '%s' FAILED. Reason: %v. "+
				"The focus mode could not be executed. Do NOT retry the same focus mode. "+
				"Proceed with a different approach using your current context and available tools.",
			identifier, err)
		invoker.AddToTimeline("[LOAD_CAPABILITY_FOCUS_MODE_FAILED]", failMsg)
		op.Feedback(failMsg)
		op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		op.SetReflectionData("focus_mode_error", err.Error())
		op.SetReflectionData("focus_mode_name", identifier)
		op.Continue()
		return
	}

	successMsg := fmt.Sprintf(
		"Focus mode '%s' completed SUCCESSFULLY. "+
			"The focused sub-loop has finished its work. "+
			"Its results are now part of your context. Proceed with your main task.",
		identifier)
	invoker.AddToTimeline("[LOAD_CAPABILITY_FOCUS_MODE_DONE]", successMsg)
	op.Feedback(successMsg)
	op.Continue()
}

// handleLoadUnknown falls back to a 1-iteration intent recognition loop.
// Adds strong timeline pressure to prevent the AI from repeating the same unknown identifier.
func handleLoadUnknown(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	ctx interface{ Done() <-chan struct{} },
	identifier string,
	op *reactloops.LoopActionHandlerOperator,
) {
	log.Infof("load_capability: identifier '%s' is unknown, falling back to capability search", identifier)

	// Mark this identifier as "failed unknown" to detect loops
	failedKey := "_load_cap_failed_unknown_" + identifier
	prevFailed := loop.Get(failedKey)
	if prevFailed != "" {
		blockMsg := fmt.Sprintf(
			"BLOCKED: identifier '%s' has already been tried and resolved as UNKNOWN in a previous attempt. "+
				"Do NOT call load_capability with '%s' again — it will never resolve. "+
				"You MUST choose a completely different approach: use search_capabilities to discover valid names, "+
				"or use a known tool/action directly. Repeating this call wastes iterations.",
			identifier, identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_UNKNOWN_BLOCKED]", blockMsg)
		op.Feedback(blockMsg)
		op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		op.SetReflectionData("blocked_identifier", identifier)
		op.SetReflectionData("blocked_reason", "repeated_unknown_identifier")
		op.Continue()
		return
	}
	loop.Set(failedKey, "true")

	invoker.AddToTimeline("[LOAD_CAPABILITY_UNKNOWN]",
		fmt.Sprintf("Identifier '%s' not found in any registry. "+
			"Running capability search fallback. "+
			"Do NOT call load_capability('%s') again after this — use the discovered capabilities instead.",
			identifier, identifier))

	searchResult, err := reactloops.SearchCapabilities(invoker, loop, reactloops.CapabilitySearchInput{
		Query:               identifier,
		IncludeCatalogMatch: true,
	})
	if err != nil {
		log.Warnf("load_capability: capability search fallback failed: %v", err)
		failMsg := fmt.Sprintf(
			"Identifier '%s' was NOT found, and capability search FAILED (reason: %v). "+
				"Do NOT retry load_capability with '%s'. "+
				"Use search_capabilities with a descriptive query, or proceed with already-available tools.",
			identifier, err, identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_SEARCH_FAILED]", failMsg)
		op.Feedback(failMsg)
		op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		op.SetReflectionData("capability_search_error", err.Error())
		op.SetReflectionData("failed_identifier", identifier)
		op.Continue()
		return
	}

	if searchResult == nil {
		log.Warnf("load_capability: capability search result is nil")
		failMsg := fmt.Sprintf(
			"Identifier '%s' was NOT found. Capability search completed but no results could be extracted. "+
				"Do NOT retry. Use search_capabilities instead.",
			identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_SEARCH_NIL]", failMsg)
		op.Feedback(failMsg)
		op.Continue()
		return
	}

	reactloops.ApplyCapabilitySearchResult(invoker, loop, searchResult)
	compactIntent := reactloops.CompactIntentSummary(identifier)
	loop.Set("intent_analysis", compactIntent)
	if recommendedTools := renderCapabilityToolRecommendations(searchResult); recommendedTools != "" {
		loop.Set("intent_recommended_tools", recommendedTools)
	}
	if recommendedForges := renderCapabilityForgeRecommendations(searchResult); recommendedForges != "" {
		loop.Set("intent_recommended_forges", recommendedForges)
	}
	if searchResult.ContextEnrichment != "" {
		loop.Set("intent_context_enrichment", searchResult.ContextEnrichment)
	}

	matchedToolNames := strings.Join(searchResult.MatchedToolNames, ",")
	matchedForgeNames := strings.Join(searchResult.MatchedForgeNames, ",")
	matchedSkillNames := strings.Join(searchResult.MatchedSkillNames, ",")

	log.Infof("load_capability: capability search fallback completed, tools=%s, forges=%s, skills=%s",
		matchedToolNames, matchedForgeNames, matchedSkillNames)

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Identifier '%s' was NOT found. Do NOT retry load_capability(%q).\n", identifier, identifier))
	summary.WriteString(fmt.Sprintf("未找到能力名：%s\n", identifier))
	if compactIntent != "" {
		summary.WriteString("意图：" + compactIntent + "\n")
	}
	if tools := reactloops.CompactCapabilityNames(matchedToolNames, 3); tools != "" {
		summary.WriteString("可用工具：" + tools + "\n")
	}
	if forges := reactloops.CompactCapabilityNames(matchedForgeNames, 3); forges != "" {
		summary.WriteString("可用蓝图：" + forges + "\n")
	}
	if skills := reactloops.CompactCapabilityNames(matchedSkillNames, 3); skills != "" {
		summary.WriteString("可用技能：" + skills + "\n")
	}
	summary.WriteString(fmt.Sprintf("不要再次 load_capability(%q)，请改用以上正确名称。", identifier))

	invoker.AddToTimeline("[LOAD_CAPABILITY_SEARCH_DONE]",
		fmt.Sprintf("能力候选已识别：%s | 工具[%s] 蓝图[%s] 技能[%s] | 不要再次 load_capability(%s)",
			compactIntent,
			reactloops.CompactCapabilityNames(matchedToolNames, 2),
			reactloops.CompactCapabilityNames(matchedForgeNames, 2),
			reactloops.CompactCapabilityNames(matchedSkillNames, 2),
			identifier))

	op.Feedback(summary.String())
	op.Continue()
}
