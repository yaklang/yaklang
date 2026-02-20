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
	invoker.AddToTimeline("[LOAD_CAPABILITY_VERIFIED]",
		fmt.Sprintf("identifier='%s' resolved as '%s'", identifier, resolved.IdentityType))
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
func handleLoadForge(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	ctx interface{ Done() <-chan struct{} },
	identifier string,
	op *reactloops.LoopActionHandlerOperator,
) {
	log.Infof("load_capability: dispatching '%s' as blueprint/forge", identifier)
	invoker.AddToTimeline("[LOAD_CAPABILITY_FORGE]",
		fmt.Sprintf("Starting AI Blueprint '%s' in async mode", identifier))

	op.RequestAsyncMode()

	task := op.GetTask()
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
		invoker.AddToTimeline("[LOAD_CAPABILITY_FOCUS_MODE_ERROR]",
			fmt.Sprintf("Focus mode '%s' failed: %v", identifier, err))
		op.Feedback(fmt.Sprintf("Focus mode '%s' execution failed: %v. Proceeding with current context.", identifier, err))
		op.Continue()
		return
	}
	if !ok {
		log.Warnf("load_capability: focus mode '%s' returned not ok", identifier)
		op.Feedback(fmt.Sprintf("Focus mode '%s' completed but returned unsuccessful. Proceeding with current context.", identifier))
		op.Continue()
		return
	}

	invoker.AddToTimeline("[LOAD_CAPABILITY_FOCUS_MODE_DONE]",
		fmt.Sprintf("Focus mode '%s' completed successfully", identifier))
	op.Feedback(fmt.Sprintf("Focus mode '%s' has completed successfully. Proceeding with your task.", identifier))
	op.Continue()
}

// handleLoadUnknown falls back to a 1-iteration intent recognition loop.
func handleLoadUnknown(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	ctx interface{ Done() <-chan struct{} },
	identifier string,
	op *reactloops.LoopActionHandlerOperator,
) {
	log.Infof("load_capability: identifier '%s' is unknown, falling back to intent recognition", identifier)
	invoker.AddToTimeline("[LOAD_CAPABILITY_UNKNOWN]",
		fmt.Sprintf("Identifier '%s' not found in any registry. Running 1-iteration intent recognition loop.", identifier))

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
		op.Feedback(fmt.Sprintf(
			"Identifier '%s' was not found and intent recognition failed: %v. "+
				"Please verify the name or try search_capabilities with a descriptive query.",
			identifier, err))
		op.Continue()
		return
	}
	if !ok {
		log.Warnf("load_capability: intent loop fallback returned not ok")
		op.Feedback(fmt.Sprintf(
			"Identifier '%s' was not found and intent recognition produced no results. "+
				"Try search_capabilities with a more descriptive query.",
			identifier))
		op.Continue()
		return
	}
	if intentLoop == nil {
		log.Warnf("load_capability: intent loop reference is nil")
		op.Feedback(fmt.Sprintf(
			"Identifier '%s' was not found. Intent recognition completed but results could not be extracted.",
			identifier))
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
	summary.WriteString(fmt.Sprintf("'%s' was not found as a direct match. "+
		"The system ran intent recognition and discovered the following:\n\n", identifier))
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
	summary.WriteString("Discovered capabilities are now available in your context. Use the appropriate action to proceed.\n")

	invoker.AddToTimeline("[LOAD_CAPABILITY_INTENT_DONE]",
		fmt.Sprintf("Intent fallback completed: tools=[%s], forges=[%s], skills=[%s]",
			matchedToolNames, matchedForgeNames, matchedSkillNames))

	op.Feedback(summary.String())
	op.Continue()
}
