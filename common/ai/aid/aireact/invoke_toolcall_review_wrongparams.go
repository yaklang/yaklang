package aireact

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// _invokeToolCall_ReviewWrongParamForTask regenerates parameters against the
// task that owns this tool call. Batch callers must pass their child/batch task
// explicitly: consulting ReAct.currentTask here would allow another concurrent
// call to silently change the user query used by this review.
func (r *ReAct) _invokeToolCall_ReviewWrongParamForTask(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	tool *aitool.Tool,
	old aitool.InvokeParams,
	extraPrompt string,
) (aitool.InvokeParams, error) {
	// Check context at the beginning
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	input := ""
	if task != nil {
		input = task.GetUserInput()
	}
	if extraPrompt != "" {
		input = input + "\n\n Extra:\n\n" + extraPrompt
	}
	r.AddToTimeline(
		"re-generate-tool-params",
		fmt.Sprintf("Regenerating parameters for tool: %s", tool.Name),
	)
	promptMeta, err := r.promptManager.GenerateReGenerateToolParamsPromptWithMeta(input, old, tool)
	if err != nil {
		return nil, err
	}
	var result = make(aitool.InvokeParams)
	transErr := aicommon.CallAITransaction(
		r.config,
		promptMeta.Prompt,
		r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			// Build action maker options for AITAG support
			var actionOpts []aicommon.ActionMakerOption
			if promptMeta.Nonce != "" && len(promptMeta.ParamNames) > 0 {
				actionOpts = append(actionOpts, aicommon.WithActionNonce(promptMeta.Nonce))
				// Register AITAG handlers for each parameter
				for _, paramName := range promptMeta.ParamNames {
					tagName := fmt.Sprintf("TOOL_PARAM_%s", paramName)
					actionOpts = append(actionOpts, aicommon.WithActionTagToKey(tagName, fmt.Sprintf("__aitag__%s", paramName)))
				}
			}

			action, err := aicommon.ExtractValidActionFromStream(
				ctx,
				rsp.GetOutputStreamReader("call-tools", true, r.Emitter),
				"call-tool",
				actionOpts...,
			)
			if err != nil {
				r.AddToTimeline("err", fmt.Sprintf(
					"generate tool params failed: %v", err,
				))
				return err
			}
			switch string(action.ActionType()) {
			case "call-tool":
				// First, get params from JSON
				result = action.GetInvokeParams("params")

				// Then, merge AITAG params (they take precedence over JSON params)
				if len(promptMeta.ParamNames) > 0 {
					for _, paramName := range promptMeta.ParamNames {
						aitagKey := fmt.Sprintf("__aitag__%s", paramName)
						if aitagValue := action.GetString(aitagKey); aitagValue != "" {
							result.Set(paramName, aitagValue)
						}
					}
				}

				ok, reasons := tool.ValidateParams(result)
				if !ok {
					err := utils.Errorf("invalid tool params: %v", reasons)
					r.AddToTimeline("err", fmt.Sprintf(
						"generate tool params failed: %v", err,
					))
					return err
				}
				r.AddToTimeline("re-generate-tool-params", fmt.Sprintf(
					"Result:\n%v", utils.PrefixLines(result.Dump(), "  ")))
				return nil
			default:
				err := utils.Errorf("cannot handle action type: %s", action.ActionType())
				r.AddToTimeline("err", fmt.Sprintf(
					"generate tool params failed: %v", err,
				))
				return err
			}
		},
		aicommon.WithAIRequest_CallerLabel("toolcall-review-wrongparams"),
		aicommon.WithAIRequest_Context(ctx),
	)
	if transErr != nil {
		return nil, transErr
	}
	return result, nil
}
