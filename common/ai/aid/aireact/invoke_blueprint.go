package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type AIForgeReviewSuggestion struct {
	Value            string `json:"value"`
	Prompt           string `json:"prompt"`
	PromptEnglish    string `json:"prompt_english"`
	AllowExtraPrompt bool   `json:"allow_extra_prompt"`
}

var aiforgeReviewSelector = []*AIForgeReviewSuggestion{
	{
		Value:         "continue",
		Prompt:        "同意参数，继续执行",
		PromptEnglish: "Agree with the parameters, continue execution",
	},
	{
		Value:            "modify_params",
		Prompt:           "修改参数",
		PromptEnglish:    "Modify the parameters",
		AllowExtraPrompt: true,
	},
	{
		Value:            "change_blueprint",
		Prompt:           "更换AI应用",
		PromptEnglish:    "Change to another AI Forge",
		AllowExtraPrompt: true,
	},
	{
		Value:         "cancel",
		Prompt:        "取消执行",
		PromptEnglish: "Cancel the execution",
	},
}

func (r *ReAct) reviewAIForge(
	ins *schema.AIForge,
	invokeParams aitool.InvokeParams,
) (*schema.AIForge, aitool.InvokeParams, bool, error) {
	// reivew
	epm := r.config.epm
	ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	reqs := map[string]any{
		"id":                 ep.GetId(),
		"forge_name":         ins.ForgeName,
		"forge_desc":         ins.Description,
		"forge_verbose_name": ins.ForgeVerboseName,
		"forge_params":       invokeParams,
		"selectors":          aiforgeReviewSelector,
	}
	ep.SetReviewMaterials(reqs)
	r.Emitter.EmitInteractiveJSON(
		ep.GetId(),
		schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE,
		"review-require", reqs,
	)
	r.AddToTimeline("review-ai-blueprint-params", aitool.InvokeParams(reqs).Dump())
	r.config.DoWaitAgree(r.config.GetContext(), ep)
	reviewParams := ep.GetParams()

	releaseOnce := utils.NewOnce()
	release := func() {
		releaseOnce.Do(func() {
			// continue
			r.config.EmitInteractiveRelease(ep.GetId(), reviewParams)
			r.config.CallAfterInteractiveEventReleased(ep.GetId(), reviewParams)
			r.config.CallAfterReview(ep.GetSeq(), fmt.Sprintf(
				"determite aiforge[%v]'s params is proper? why?",
				ins.ForgeName,
			), reviewParams)
		})
	}
	defer func() {
		release()
	}()

	suggestion := reviewParams.GetAnyToString("suggestion")
	switch suggestion {
	case "cancel":
		r.Emitter.EmitWarning("AI Forge execution cancelled by user")
		r.EmitResult("AI智能应用执行已被用户取消(cancelled by user), 用户仍然可以继续对话, 或重新发起智能应用调用")
		return nil, nil, false, utils.Error("ai forge execution cancelled by user")
	case "modify_params":
		return r.invokeBlueprintReviewModifyParams(ins, invokeParams, reviewParams, release)
	case "change_blueprint":
		return r.invokeBlueprintReviewChangeBlueprint(ins, invokeParams, release)
	case "continue":
		return ins, invokeParams, false, nil
	default:
		return nil, nil, false, utils.Error("unknown suggestion from review: " + suggestion)
	}
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
				r.config.GetContext(),
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

	ins, forgeParams, _, err = r.reviewAIForge(ins, forgeParams)
	if err != nil {
		return nil, nil, err
	}
	if utils.IsNil(forgeParams) {
		r.Emitter.EmitError("ai-forge params is nil after review")
		return nil, nil, utils.Errorf("ai-forge params is nil after review")
	}
	return ins, forgeParams, nil
}
