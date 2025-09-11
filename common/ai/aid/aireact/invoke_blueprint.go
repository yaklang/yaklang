package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) invokeBlueprint(forgeName string) (*schema.AIForge, aitool.InvokeParams, error) {
	manager := r.config.aiBlueprintManager
	ins, err := manager.GetAIForge(forgeName)
	if err != nil {
		return nil, nil, utils.Errorf("get ai forge [%v] failed", err)
	}
	schema, err := manager.GenerateAIJSONSchemaFromSchemaAIForge(ins)
	if err != nil {
		return nil, nil, utils.Errorf("generate ai json schema from schema ai forge failed: %v", err)
	}
	prompt, err := r.promptManager.GenerateAIBlueprintForgeParamsPrompt(schema)
	if err != nil {
		return nil, nil, utils.Errorf("generate prompt (for ai-forge) failed: %v", err)
	}

	var forgeParams = make(aitool.InvokeParams)
	err = aicommon.CallAITransaction(
		r.config, prompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("call-forge", false, r.config.GetEmitter())
			action, err := aicommon.ExtractActionFromStream(
				stream, "call-ai-blueprint",
			)
			if err != nil {
				return utils.Errorf("extract action from call-ai-blueprint failed: %v", err)
			}
			forgeParams = action.GetInvokeParams("params")
			if len(forgeParams) <= 0 {
				return utils.Error("forge params is empty, require at least one param")
			}
			return nil
		},
	)
	if err != nil {
		return nil, nil, err
	}
	return ins, forgeParams, nil
}
