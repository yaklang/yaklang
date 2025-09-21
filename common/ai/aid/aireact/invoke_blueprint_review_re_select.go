package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) invokeBlueprintReviewChangeBlueprint(
	ins *schema.AIForge,
	invokeParams aitool.InvokeParams,
	release func(),
) (*schema.AIForge, aitool.InvokeParams, bool, error) {
	defer release()

	// Get available AI Forge blueprints
	forgeList := r.promptManager.GetAvailableAIForgeBlueprints()
	if forgeList == "" {
		r.addToTimeline("note", "No available AI Forge blueprints for re-selection")
		return ins, invokeParams, false, utils.Errorf("no available AI Forge blueprints")
	}

	// Generate change blueprint prompt
	prompt, err := r.promptManager.GenerateChangeAIBlueprintPrompt(
		ins, forgeList, invokeParams, "",
	)
	if err != nil {
		return nil, nil, false, utils.Errorf("generate change blueprint prompt failed: %v", err)
	}

	release()

	var selectedBlueprintName string
	err = aicommon.CallAITransaction(
		r.config, prompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			reader := rsp.GetOutputStreamReader(
				"change-blueprint-selection",
				false,
				r.Emitter,
			)
			action, err := aicommon.ExtractActionFromStream(
				reader, "change-ai-blueprint",
			)
			if err != nil {
				return utils.Errorf("extract action from change-ai-blueprint failed: %v", err)
			}
			selectedBlueprintName = action.GetString("new_blueprint")
			reasoning := action.GetString("reasoning")
			r.addToTimeline("blueprint-selection",
				fmt.Sprintf("Selected new blueprint '%s', reasoning: %s",
					selectedBlueprintName, reasoning))
			return nil
		},
	)
	if err != nil {
		return nil, nil, false, err
	}

	release()

	// Find the selected blueprint
	forges, err := r.config.aiBlueprintManager.Query(r.config.GetContext())
	if err != nil {
		return nil, nil, false, utils.Errorf("query AI Forge blueprints failed: %v", err)
	}

	var selectedForge *schema.AIForge
	for _, forge := range forges {
		if forge.ForgeName == selectedBlueprintName {
			selectedForge = forge
			break
		}
	}

	if selectedForge == nil {
		return nil, nil, false, utils.Errorf("selected blueprint '%s' not found", selectedBlueprintName)
	}

	release()

	// Generate new parameters for the selected blueprint
	manager := r.config.aiBlueprintManager
	forgeSchema, err := manager.GenerateAIJSONSchemaFromSchemaAIForge(selectedForge)
	if err != nil {
		return nil, nil, false, utils.Errorf("generate ai json schema for selected forge failed: %v", err)
	}

	prompt, err = r.promptManager.GenerateAIBlueprintForgeParamsPrompt(selectedForge, forgeSchema)
	if err != nil {
		return nil, nil, false, utils.Errorf("generate prompt for new blueprint failed: %v", err)
	}

	var newParams = make(aitool.InvokeParams)
	err = aicommon.CallAITransaction(
		r.config, prompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("call-new-forge", false, r.Emitter)
			action, err := aicommon.ExtractActionFromStream(
				stream, "call-ai-blueprint",
			)
			if err != nil {
				return utils.Errorf("extract action from call-ai-blueprint for new forge failed: %v", err)
			}
			newParams = action.GetInvokeParams("params")
			if len(newParams) <= 0 {
				return utils.Error("new forge params is empty, require at least one param")
			}
			return nil
		},
	)
	if err != nil {
		return nil, nil, false, err
	}

	// Return the new blueprint with newly generated parameters
	// The third parameter (bool) indicates whether further review is needed - false means we're done
	return selectedForge, newParams, false, nil
}
