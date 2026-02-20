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
	ActionType: schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY,
	Description: "Load a capability by identifier. Automatically detects whether the identifier is a " +
		"tool, AI blueprint (forge), skill, or focus mode loop, and dispatches accordingly. " +
		"Use this when you know the exact name of a capability but are unsure of its type.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"identifier",
			aitool.WithParam_Description(
				"The exact name of the capability to load. "+
					"This can be a tool name (e.g. 'check-yaklang-syntax'), "+
					"an AI blueprint/forge name (e.g. 'code_generator'), "+
					"a skill name, or a focus mode loop name. "+
					"The system will automatically detect the type and handle it."),
			aitool.WithParam_Required(true),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "identifier", AINodeId: "load_capability"},
	},
	ActionVerifier: loadCapabilityVerifier,
	ActionHandler:  loadCapabilityHandler,
}

func loadCapabilityVerifier(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	identifier := strings.TrimSpace(action.GetString("identifier"))
	if identifier == "" {
		identifier = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("identifier"))
	}
	if identifier == "" {
		return utils.Error("load_capability action requires 'identifier' parameter")
	}

	resolved := loop.ResolveIdentifier(identifier)
	loop.Set("_load_cap_identifier", identifier)
	loop.Set("_load_cap_resolved_type", string(resolved.IdentityType))
	loop.Set("_load_cap_suggestion", resolved.Suggestion)

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

	switch resolvedType {
	case aicommon.ResolvedAs_Tool:
		handleLoadTool(loop, invoker, ctx, identifier, op)
	case aicommon.ResolvedAs_Forge:
		handleLoadForge(loop, invoker, ctx, identifier, op)
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
		verifyResult.CompletedTaskIndex, verifyResult.NextMovements,
	)
	if verifyResult.Satisfied {
		op.Exit()
		return
	}
	feedbackMsg := fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning)
	if verifyResult.NextMovements != "" {
		feedbackMsg += fmt.Sprintf("\nNext Steps: %s", verifyResult.NextMovements)
	}
	op.Feedback(feedbackMsg)
	op.Continue()
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

	op.RequestAsyncMode()

	task = op.GetTask()
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
	op.Feedback(fmt.Sprintf(
		"Skill '%s' has been loaded into the context. "+
			"The SKILL.md content and file tree are now displayed in the SKILLS_CONTEXT section of your prompt. "+
			"Read the skill content from your prompt's View Window and proceed with the task. "+
			"Do NOT load this skill again.",
		identifier))
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

	ok, err := invoker.ExecuteLoopTaskIF(identifier, subTask, opts...)
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
	if !ok {
		log.Warnf("load_capability: focus mode '%s' returned not ok", identifier)
		failMsg := fmt.Sprintf(
			"Focus mode '%s' completed but returned UNSUCCESSFUL. "+
				"The sub-loop did not produce a satisfactory outcome. "+
				"Do NOT retry the same focus mode with the same input. "+
				"Proceed with a different strategy.",
			identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_FOCUS_MODE_UNSUCCESSFUL]", failMsg)
		op.Feedback(failMsg)
		op.SetReflectionLevel(reactloops.ReflectionLevel_Deep)
		op.SetReflectionData("focus_mode_status", "unsuccessful")
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
	log.Infof("load_capability: identifier '%s' is unknown, falling back to intent recognition", identifier)

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
			"Running 1-iteration intent recognition fallback. "+
			"Do NOT call load_capability('%s') again after this — use the discovered capabilities instead.",
			identifier, identifier))

	cfg := invoker.GetConfig()
	taskCtx := cfg.GetContext()
	task := loop.GetCurrentTask()
	if task != nil {
		taskCtx = task.GetContext()
	}

	intentTask := aicommon.NewStatefulTaskBase(
		invoker.GetCurrentTaskId()+"_load_cap_intent",
		identifier,
		taskCtx,
		cfg.GetEmitter(),
	)

	originOptions := cfg.OriginOptions()
	var opts []any
	for _, option := range originOptions {
		opts = append(opts, option)
	}

	var intentLoop *reactloops.ReActLoop
	opts = append(opts, reactloops.WithOnLoopInstanceCreated(func(l *reactloops.ReActLoop) {
		intentLoop = l
	}))

	ok, err := invoker.ExecuteLoopTaskIF(schema.AI_REACT_LOOP_NAME_INTENT, intentTask, opts...)
	if err != nil {
		log.Warnf("load_capability: intent loop fallback failed: %v", err)
		failMsg := fmt.Sprintf(
			"Identifier '%s' was NOT found, and intent recognition FAILED (reason: %v). "+
				"Do NOT retry load_capability with '%s'. "+
				"Use search_capabilities with a descriptive query, or proceed with already-available tools.",
			identifier, err, identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_INTENT_FAILED]", failMsg)
		op.Feedback(failMsg)
		op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		op.SetReflectionData("intent_error", err.Error())
		op.SetReflectionData("failed_identifier", identifier)
		op.Continue()
		return
	}
	if !ok {
		log.Warnf("load_capability: intent loop fallback returned not ok")
		failMsg := fmt.Sprintf(
			"Identifier '%s' was NOT found, and intent recognition produced NO useful results. "+
				"Do NOT retry load_capability with '%s'. "+
				"Use search_capabilities with a different, more descriptive query instead.",
			identifier, identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_INTENT_NO_RESULT]", failMsg)
		op.Feedback(failMsg)
		op.SetReflectionLevel(reactloops.ReflectionLevel_Deep)
		op.SetReflectionData("failed_identifier", identifier)
		op.Continue()
		return
	}
	if intentLoop == nil {
		log.Warnf("load_capability: intent loop reference is nil")
		failMsg := fmt.Sprintf(
			"Identifier '%s' was NOT found. Intent recognition completed but results could not be extracted. "+
				"Do NOT retry. Use search_capabilities instead.",
			identifier)
		invoker.AddToTimeline("[LOAD_CAPABILITY_INTENT_NIL]", failMsg)
		op.Feedback(failMsg)
		op.Continue()
		return
	}

	intentAnalysis := intentLoop.Get("intent_analysis")
	recommendedTools := intentLoop.Get("recommended_tools")
	recommendedForges := intentLoop.Get("recommended_forges")
	contextEnrichment := intentLoop.Get("context_enrichment")
	matchedToolNames := intentLoop.Get("matched_tool_names")
	matchedForgeNames := intentLoop.Get("matched_forge_names")
	matchedSkillNames := intentLoop.Get("matched_skill_names")

	log.Infof("load_capability: intent fallback completed, analysis=%d bytes, tools=%s, forges=%s, skills=%s",
		len(intentAnalysis), matchedToolNames, matchedForgeNames, matchedSkillNames)

	if intentAnalysis != "" {
		loop.Set("intent_analysis", intentAnalysis)
		invoker.AddToTimeline("load_capability_intent_analysis", intentAnalysis)
	}
	if recommendedTools != "" {
		loop.Set("intent_recommended_tools", recommendedTools)
	}
	if recommendedForges != "" {
		loop.Set("intent_recommended_forges", recommendedForges)
	}
	if contextEnrichment != "" {
		loop.Set("intent_context_enrichment", contextEnrichment)
	}

	populateExtraCapabilitiesFromIntent(invoker, loop, matchedToolNames, matchedForgeNames, matchedSkillNames)

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("## Capability Search Results for: %s\n\n", identifier))
	summary.WriteString(fmt.Sprintf("'%s' was NOT found as a direct match. "+
		"Intent recognition discovered the following alternatives:\n\n", identifier))
	if intentAnalysis != "" {
		summary.WriteString(intentAnalysis)
		summary.WriteString("\n\n")
	}
	if recommendedTools != "" {
		summary.WriteString("**Recommended Tools**: " + recommendedTools + "\n")
	}
	if recommendedForges != "" {
		summary.WriteString("**Recommended Forges**: " + recommendedForges + "\n")
	}
	if matchedSkillNames != "" {
		summary.WriteString("**Matched Skills**: " + matchedSkillNames + "\n")
	}
	if contextEnrichment != "" {
		summary.WriteString("\n" + contextEnrichment)
	}
	summary.WriteString("\n---\n")
	summary.WriteString(fmt.Sprintf(
		"IMPORTANT: '%s' is confirmed as NOT a valid capability name. "+
			"Do NOT call load_capability('%s') again. "+
			"Use the discovered capabilities above with their correct names, or choose a different approach.\n",
		identifier, identifier))

	invoker.AddToTimeline("[LOAD_CAPABILITY_INTENT_DONE]",
		fmt.Sprintf("Intent fallback completed for '%s': tools=[%s], forges=[%s], skills=[%s]. "+
			"Do NOT retry load_capability('%s').",
			identifier, matchedToolNames, matchedForgeNames, matchedSkillNames, identifier))

	op.Feedback(summary.String())
	op.Continue()
}
