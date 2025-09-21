package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type AIForgeReviewSuggestion struct {
	Value             string `json:"value"`
	Suggestion        string `json:"suggestion"`
	SuggestionEnglish string `json:"suggestion_english"`
}

var aiforgeReviewSelector = []*AIForgeReviewSuggestion{
	{
		Value:             "continue",
		Suggestion:        "同意参数，继续执行",
		SuggestionEnglish: "Agree with the parameters, continue execution",
	},
	{
		Value:             "modify_params",
		Suggestion:        "修改参数",
		SuggestionEnglish: "Modify the parameters",
	},
	{
		Value:             "change_aiforge",
		Suggestion:        "更换AI应用",
		SuggestionEnglish: "Change to another AI Forge",
	},
	{
		Value:             "cancel",
		Suggestion:        "取消执行",
		SuggestionEnglish: "Cancel the execution",
	},
}

func (r *ReAct) invokeBlueprint(forgeName string) (*schema.AIForge, aitool.InvokeParams, error) {
	manager := r.config.aiBlueprintManager
	ins, err := manager.GetAIForge(forgeName)
	if err != nil {
		return nil, nil, utils.Errorf("get ai forge [%v] failed", err)
	}

	forgeSchema, err := manager.GenerateAIJSONSchemaFromSchemaAIForge(ins)
	if err != nil {
		return nil, nil, utils.Errorf("generate ai json schema from schema ai forge failed: %v", err)
	}
	prompt, err := r.promptManager.GenerateAIBlueprintForgeParamsPrompt(ins, forgeSchema)
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

	// reivew
	epm := r.config.epm
	ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	reqs := map[string]any{
		"id":                 ep.GetId(),
		"forge_name":         ins.ForgeName,
		"forge_desc":         ins.Description,
		"forge_verbose_name": ins.ForgeVerboseName,
		"forge_params":       forgeParams,
		"selectors":          aiforgeReviewSelector,
	}
	ep.SetReviewMaterials(reqs)
	r.Emitter.EmitInteractiveJSON(
		ep.GetId(),
		schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE,
		"review-require", reqs,
	)
	r.config.DoWaitAgree(r.config.GetContext(), ep)

	params := ep.GetParams()

	suggestion := params.GetAnyToString("suggestion")
	switch suggestion {
	case "cancel":
		r.Emitter.EmitInfo("AI Forge execution cancelled by user")
		return nil, nil, utils.Error("ai forge execution cancelled by user")
	case "modify_params":
		return nil, nil, utils.Error("ai forge execution cancelled by user, todo: modify params not implemented yet")
	case "change_aiforge":
		return nil, nil, utils.Error("ai forge execution cancelled by user, todo: change aiforge not implemented yet")
	case "continue":
		r.config.EmitInteractiveRelease(ep.GetId(), params)
		r.config.CallAfterInteractiveEventReleased(ep.GetId(), params)
		r.config.CallAfterReview(ep.GetSeq(), fmt.Sprintf(
			"determite aiforge[%v]'s params is proper? why?",
			ins.ForgeName,
		), params)
		if utils.IsNil(params) {
			r.Emitter.EmitError("ai-forge params is nil after review")
			return nil, nil, utils.Errorf("ai-forge params is nil after review")
		}

		return ins, forgeParams, nil
	default:
		return nil, nil, utils.Error("unknown suggestion from review: " + suggestion)
	}
}
