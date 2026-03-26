package loopinfra

import (
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_toolRequireAndCall = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description: "申请工具调用，执行这个 @action 会进入工具申请流程，查看工具教程以及文档，来生成参数",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description(`MUST set in {"@action": "require_tool", ... }. 根据上下文信息，提供你想要申请的工具名，只说明工具名即可，严禁包含参数.`),
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		payload := action.GetString("tool_require_payload")
		if payload == "" {
			payload = action.GetInvokeParams("next_action").GetString("tool_require_payload")
		}
		if payload == "" {
			return utils.Error("tool_require_payload is required for ActionRequireTool but empty")
		}
		loop.Set("tool_require_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		toolPayload := loop.Get("tool_require_payload")
		if toolPayload == "" {
			operator.Feedback(utils.Error("tool_require_payload is required for ActionRequireTool but empty"))
			return
		}
		invoker := loop.GetInvoker()
		ctx := invoker.GetConfig().GetContext()
		t := loop.GetCurrentTask()
		if t != nil {
			ctx = t.GetContext()
		}

		// loading file or tool
		pr, pw := utils.NewPipe()
		pw.WriteString("loading tool: ")
		pw.WriteString(toolPayload)
		pw.WriteString("...")
		closeOnce := new(sync.Once)
		closeStatusPipe := func() {
			closeOnce.Do(func() {
				pw.Close()
			})
		}
		loop.GetEmitter().EmitDefaultStreamEvent("load_tool", pr, operator.GetTask().GetId())
		defer closeStatusPipe()

		toolIns, err := loop.GetConfig().GetAiToolManager().GetToolByName(toolPayload)
		if err != nil {
			pw.WriteString(fmt.Sprintf("Error: %v", err.Error()))
		} else {
			pw.WriteString(utils.MustRenderTemplate(
				`done! {{ .Name }}{{ if .VerboseName}}({{.VerboseName}}){{ end }} is prepared`,
				map[string]interface{}{
					"Name":        toolIns.GetName(),
					"VerboseName": toolIns.GetVerboseName(),
				}),
			)
		}

		result, directly, err := invoker.ExecuteToolRequiredAndCall(ctx, toolPayload)
		if err != nil {
			// Record the error in timeline and allow AI to retry with a different tool or approach
			errMsg := fmt.Sprintf("Tool '%s' execution failed: %v.", toolPayload, err)
			invoker.AddToTimeline("[TOOL_EXECUTION_ERROR]", errMsg)

			// Try to resolve the identifier - it might be a forge or skill, not a tool
			resolved := loop.ResolveIdentifier(toolPayload)
			if !resolved.IsUnknown() && resolved.IdentityType != aicommon.ResolvedAs_Tool {
				// The identifier exists as a different type - provide clear guidance
				invoker.AddToTimeline("identifier_resolved", resolved.Suggestion)
				operator.Feedback(errMsg + "\n\n" + resolved.Suggestion)
			} else {
				operator.Feedback(errMsg + " Please try a different tool or approach.")
			}

			// Set reflection level to help AI understand the failure
			operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
			operator.SetReflectionData("tool_error", err.Error())
			operator.SetReflectionData("tool_name", toolPayload)
			operator.SetReflectionData("resolved_type", string(resolved.IdentityType))
			// Continue the loop to give AI a chance to retry
			operator.Continue()
			return
		}
		if directly {
			answer, err := invoker.DirectlyAnswer(ctx, "在上一次工具调用中，用户中断了工具执行，要求直接回答一些问题。一般这种情况出现在用户认为这个任务不应该使用工具或者工具无法满足需求的情况下。", nil)
			if err != nil {
				operator.Fail(utils.Error("DirectlyAnswer fail, reason: " + err.Error()))
				return
			}
			invoker.AddToTimeline("directly-answer", answer)
			operator.Exit()
			return
		}

		if result == nil {
			msg := fmt.Sprintf("ExecuteToolRequiredAndCall[%v] returned nil result", toolPayload)
			invoker.AddToTimeline("error", msg)
			operator.Continue()
			return
		}

		if result.Error != "" {
			invoker.AddToTimeline("call["+toolPayload+"] error", result.Error)
		}

		task := loop.GetCurrentTask()
		verifyResult, err := invoker.VerifyUserSatisfaction(ctx, task.GetUserInput(), true, toolPayload)
		if err != nil {
			operator.Fail(err)
			return
		}

		if len(verifyResult.OutputFiles) > 0 {
			cfg := loop.GetConfig()
			for _, filePath := range verifyResult.OutputFiles {
				providerName := "output_file:" + filePath
				cfg.GetContextProviderManager().RegisterTracedContent(
					providerName,
					aicommon.OutputFileContextProvider(filePath),
				)
				if emitter := cfg.GetEmitter(); emitter != nil {
					emitter.EmitPinFilename(filePath)
				}
			}
		}

		loop.PushSatisfactionRecordWithCompletedTaskIndex(verifyResult.Satisfied, verifyResult.Reasoning, verifyResult.CompletedTaskIndex, verifyResult.NextMovements)

		if verifyResult.Satisfied {
			operator.Exit()
			return
		}

		feedbackMsg := fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning)
		if summary := aicommon.FormatVerifyNextMovementsSummary(verifyResult.NextMovements); summary != "" {
			feedbackMsg += fmt.Sprintf("\nNext Steps: %s", summary)
		}
		operator.Feedback(feedbackMsg)
		operator.Continue()
	},
}
