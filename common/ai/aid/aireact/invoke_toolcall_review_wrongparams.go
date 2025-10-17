package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) _invokeToolCall_ReviewWrongParam(tool *aitool.Tool, old aitool.InvokeParams, extraPrompt string) (aitool.InvokeParams, error) {
	input := r.GetCurrentTask().GetUserInput()
	if extraPrompt != "" {
		input = input + "\n\n Extra:\n\n" + extraPrompt
	}
	r.AddToTimeline(
		"re-generate-tool-params",
		fmt.Sprintf("Regenerating parameters for tool: %s", tool.Name),
	)
	prompt, err := r.config.promptManager.GenerateReGenerateToolParamsPrompt(input, old, tool)
	if err != nil {
		return nil, err
	}
	var result = make(aitool.InvokeParams)
	transErr := aicommon.CallAITransaction(
		r.config,
		prompt,
		r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			action, err := aicommon.ExtractActionFromStream(
				rsp.GetOutputStreamReader("call-tools", true, r.Emitter),
				"call-tool",
			)
			if err != nil {
				r.AddToTimeline("err", fmt.Sprintf(
					"generate tool params failed: %v", err,
				))
				return err
			}
			switch string(action.ActionType()) {
			case "call-tool":
				result = action.GetInvokeParams("params")
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
	)
	if transErr != nil {
		return nil, transErr
	}
	return result, nil
}
