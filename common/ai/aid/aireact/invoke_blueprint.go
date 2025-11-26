package aireact

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
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
		Prompt:           "AI修改参数",
		PromptEnglish:    "ai Modify the parameters",
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
	{
		Value:         "input_params",
		Prompt:        "直接修改参数",
		PromptEnglish: "user directly modify the parameters",
	},
}

func (r *ReAct) reviewAIForge(
	ins *schema.AIForge,
	invokeParams aitool.InvokeParams,
) (*schema.AIForge, aitool.InvokeParams, bool, error) {
	// reivew
	epm := r.config.Epm
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
	case "input_params":
		inputParams := reviewParams.GetObject("params")
		return ins, inputParams, false, nil
	case "change_blueprint":
		return r.invokeBlueprintReviewChangeBlueprint(ins, invokeParams, release)
	case "continue":
		return ins, invokeParams, false, nil
	default:
		return nil, nil, false, utils.Error("unknown suggestion from review: " + suggestion)
	}
}

func (r *ReAct) invokeBlueprint(forgeName string) (*schema.AIForge, aitool.InvokeParams, error) {
	manager := r.config.AiForgeManager

	// 首先检查 Forge 是否存在
	ins, err := manager.GetAIForge(forgeName)
	if err != nil {
		// 记录详细的错误信息到 Timeline，使用明显的标识符
		resultMsg := fmt.Sprintf("无法找到 AI 智能应用 '%s'，请检查应用名称是否正确。可用的应用可以通过工具搜索查看。", forgeName)
		r.AddToTimeline("[BLUEPRINT_NOT_FOUND]", fmt.Sprintf("AI Blueprint '%s' does not exist. Error: %v\n%s", forgeName, err, resultMsg))
		r.Emitter.EmitError(fmt.Sprintf("AI Blueprint '%s' not found", forgeName))
		return nil, nil, utils.Errorf("AI Blueprint '%s' not found: %v", forgeName, err)
	}

	// 验证 Forge 实例的完整性
	if ins == nil {
		r.AddToTimeline("[BLUEPRINT_NULL_INSTANCE]", fmt.Sprintf(
			"AI Blueprint '%s' returned nil instance. 配置异常可能导致无法执行。", forgeName))
		r.Emitter.EmitError(fmt.Sprintf("AI Blueprint '%s' configuration error", forgeName))
		return nil, nil, utils.Errorf("AI Blueprint '%s' instance is nil", forgeName)
	}

	// 记录成功找到 Forge
	r.AddToTimeline("[BLUEPRINT_FOUND]", fmt.Sprintf("AI Blueprint: %s (%s)", ins.ForgeName, ins.ForgeVerboseName))

	forgeSchema, err := manager.GenerateAIJSONSchemaFromSchemaAIForge(ins)
	if err != nil {
		r.AddToTimeline("[BLUEPRINT_SCHEMA_ERROR]", fmt.Sprintf("Failed to generate schema for '%s'", forgeName))
		r.Emitter.EmitError(fmt.Sprintf("Failed to generate schema for AI Blueprint '%s'", forgeName))
		return nil, nil, utils.Errorf("generate ai json schema from schema ai forge failed: %v", err)
	}

	prompt, err := r.promptManager.GenerateAIBlueprintForgeParamsPrompt(ins, forgeSchema)
	if err != nil {
		r.AddToTimeline("[BLUEPRINT_PROMPT_ERROR]", fmt.Sprintf("Failed to generate prompt for '%s'", forgeName))
		r.Emitter.EmitError(fmt.Sprintf("Failed to generate prompt for AI Blueprint '%s'", forgeName))
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
				r.AddToTimeline("[BLUEPRINT_PARAM_EXTRACT_FAILED]",
					fmt.Sprintf("Failed to extract parameters for '%s': %v", forgeName, err))
				return utils.Errorf("extract action from call-ai-blueprint failed: %v", err)
			}
			forgeParams = action.GetInvokeParams("params")
			if len(forgeParams) <= 0 {
				r.AddToTimeline("[BLUEPRINT_EMPTY_PARAMS]",
					fmt.Sprintf("AI Blueprint '%s' returned empty parameters", forgeName))
				return utils.Error("forge params is empty, require at least one param")
			}
			// 记录成功提取参数
			r.AddToTimeline("[BLUEPRINT_PARAMS_READY]",
				fmt.Sprintf("Parameters for '%s': %v", forgeName, utils.ShrinkString(utils.InterfaceToString(forgeParams), 200)))
			return nil
		},
	)
	if err != nil {
		r.Emitter.EmitError(fmt.Sprintf("Failed to prepare AI Blueprint '%s': %v", forgeName, err))
		return nil, nil, err
	}

	// Ensure user original input is preserved in forge parameters before review
	// This prevents context loss when AI rewrites the query parameter
	currentTask := r.GetCurrentTask()
	if currentTask != nil {
		userOriginalInput := currentTask.GetUserInput()
		if userOriginalInput != "" && forgeParams != nil {
			// Check if forgeParams contains user original input
			forgeParamsStr := utils.InterfaceToString(forgeParams)
			if !strings.Contains(forgeParamsStr, userOriginalInput) {
				// User original input is not in forge params, need to append it
				log.Infof("user original input not found in forge params before review, appending it to preserve context")

				// Add user original input as a separate field
				nonce := utils.RandStringBytes(4)
				forgeParams["user_original_query"] = userOriginalInput

				// If there's a "query" field, enhance it with user original input
				if queryVal, exists := forgeParams["query"]; exists {
					queryStr := utils.InterfaceToString(queryVal)
					enhancedQuery := utils.MustRenderTemplate(`
<|用户原始需求_{{.nonce}}|>
{{ .UserOriginalInput }}
<|用户原始需求_END_{{.nonce}}|>
--- 
{{ .AIGeneratedQuery }}
`,
						map[string]any{
							"nonce":             nonce,
							"UserOriginalInput": userOriginalInput,
							"AIGeneratedQuery":  queryStr,
						})
					forgeParams["query"] = enhancedQuery
					log.Infof("enhanced forge query param with user original input before review")
				}
			}
		}
	}

	ins, forgeParams, _, err = r.reviewAIForge(ins, forgeParams)
	if err != nil {
		r.AddToTimeline("[BLUEPRINT_REVIEW_FAILED]", fmt.Sprintf("Review failed for '%s': %v", forgeName, err))
		return nil, nil, err
	}
	if utils.IsNil(forgeParams) {
		r.AddToTimeline("[BLUEPRINT_NIL_PARAMS_AFTER_REVIEW]", "Parameters became nil after review")
		r.Emitter.EmitError("ai-forge params is nil after review")
		return nil, nil, utils.Errorf("ai-forge params is nil after review")
	}
	return ins, forgeParams, nil
}
