package aireact

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"io"
	"time"

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

	var selectedForge *schema.AIForge
	err = aicommon.CallAITransaction(
		r.config, prompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			reader := rsp.GetOutputStreamReader(
				"change-blueprint-selection",
				false,
				r.Emitter,
			)
			action, err := aicommon.ExtractWaitableActionFromStream(
				r.config.ctx, reader,
				"change-ai-blueprint",
				[]string{}, []jsonextractor.CallbackOption{
					jsonextractor.WithRegisterFieldStreamHandler(
						"reasoning",
						func(key string, reasonReader io.Reader, parents []string) {
							var reasonBuf bytes.Buffer
							var reason = io.TeeReader(reasonReader, &reasonBuf)
							r.Emitter.EmitStreamEvent(
								"change-blueprint-reasoning",
								time.Now(),
								reason,
								r.GetCurrentTask().GetId(),
								func() {
									r.addToTimeline("blueprint-selection", "Reasoning: "+reasonBuf.String())
								},
							)
						},
					),
				},
			)
			if err != nil {
				return utils.Errorf("extract action from change-ai-blueprint failed: %v", err)
			}
			selectedBlueprintName := action.WaitString("new_blueprint")
			if selectedBlueprintName == "" {
				return utils.Error("selected blueprint name is empty, require non-empty")
			}

			selected, err := r.config.aiBlueprintManager.GetAIForge(selectedBlueprintName)
			if err != nil {
				return utils.Errorf("get selected blueprint '%s' info failed: %v", selectedBlueprintName, err)
			}
			if selected == nil {
				return utils.Errorf("selected blueprint '%s' not found", selectedBlueprintName)
			}
			selectedForge = selected
			return nil
		},
	)
	if err != nil {
		return nil, nil, false, err
	}
	if selectedForge == nil {
		return nil, nil, false, utils.Error("selected blueprint is nil after ai call")
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

	r.addToTimeline(
		"blueprint-selection",
		fmt.Sprintf("Selected new blueprint: %v \nwith params: %v", selectedForge.ForgeName, newParams))
	// Return the new blueprint with newly generated parameters
	// The third parameter (bool) indicates whether further review is needed - false means we're done
	return selectedForge, newParams, false, nil
}
