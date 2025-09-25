package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) invokeBlueprintReviewModifyParams(
	ins *schema.AIForge, invokeParams, reviewParams aitool.InvokeParams,
	release func(),
) (*schema.AIForge, aitool.InvokeParams, bool, error) {
	defer release()

	manager := r.config.aiBlueprintManager
	extraPrompt := reviewParams.GetString("extra_prompt")
	schemaString, err := manager.GenerateAIJSONSchemaFromSchemaAIForge(ins)
	if err != nil {
		return nil, nil, false, err
	}
	if extraPrompt == "" {
		extraPrompt = "请根据之前的对话内容和AI应用描述，调整参数，使其更合理。用户觉得旧的参数有问题，不满足需求，请你完善参数内容，或者给一个更详细的版本。"
	}
	prompt, err := r.promptManager.GenerateAIBlueprintForgeParamsPromptEx(
		ins, schemaString, invokeParams, extraPrompt,
	)
	if err != nil {
		return nil, nil, false, err
	}
	release()
	var newParams = make(aitool.InvokeParams)
	err = aicommon.CallAITransaction(
		r.config, prompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			reader := rsp.GetOutputStreamReader(
				"re-generate-blueprint-params",
				false,
				r.Emitter,
			)
			action, err := aicommon.ExtractActionFromStream(
				reader, "call-ai-blueprint",
			)
			if err != nil {
				return utils.Errorf("extract action from call-ai-blueprint failed: %v", err)
			}
			newParams = action.GetParams()
			if newParams.Has("@action") && newParams.Has("params") {
				newParams = newParams.GetObject("params")
			}
			return nil
		},
	)
	if err != nil {
		return nil, nil, false, err
	}
	release()
	return r.reviewAIForge(ins, newParams)
}
