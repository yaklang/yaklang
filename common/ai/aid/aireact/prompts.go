package aireact

import (
	_ "embed"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const prevUserInputTagMaxTokens = 20 * 1024

func nonce() string {
	return utils.RandAlphaNumStringBytes(5)
}

//go:embed prompts/tool-params/instruction.txt
var toolParamsInstructionText string

//go:embed prompts/tool-params/output_example.txt
var toolParamsOutputExampleText string

//go:embed prompts/tool-params/dynamic.txt
var toolParamsDynamicTemplate string

//go:embed prompts/verification/instruction.txt
var verificationInstructionText string

//go:embed prompts/verification/output_example.txt
var verificationOutputExampleText string

//go:embed prompts/verification/dynamic.txt
var verificationDynamicTemplate string

//go:embed prompts/verification/verification.json
var verificationSchemaJSON string

//go:embed prompts/answer/instruction.txt
var directlyAnswerInstructionText string

//go:embed prompts/answer/output_example.txt
var directlyAnswerOutputExampleText string

//go:embed prompts/answer/dynamic.txt
var directlyAnswerDynamicTemplate string

//go:embed prompts/tool/wrong-tool_instruction.txt
var wrongToolInstructionText string

//go:embed prompts/tool/wrong-tool_output_example.txt
var wrongToolOutputExampleText string

//go:embed prompts/tool/wrong-tool_dynamic.txt
var wrongToolDynamicTemplate string

//go:embed prompts/tool/interval-review_instruction.txt
var intervalReviewInstructionText string

//go:embed prompts/tool/interval-review_output_example.txt
var intervalReviewOutputExampleText string

//go:embed prompts/tool/interval-review_dynamic.txt
var intervalReviewDynamicTemplate string

//go:embed prompts/tool/interval-review.json
var intervalReviewSchemaJSON string

//go:embed prompts/change-blueprint/instruction.txt
var changeBlueprintInstructionText string

//go:embed prompts/change-blueprint/output_example.txt
var changeBlueprintOutputExampleText string

//go:embed prompts/change-blueprint/dynamic.txt
var changeBlueprintDynamicTemplate string

//go:embed prompts/base/base.txt
var basePrompt string

//go:embed prompts/utils/conversation_title.txt
var conversationTitlePrompt string

// PromptManager owns the embedded ReAct prompt templates and produces the
// five-section prefix + dynamic prompt for every AI call path in the engine.
type PromptManager struct {
	cpm   *aicommon.ContextProviderManager
	react *ReAct

	workdir       string
	glanceWorkdir string
}

func NewPromptManager(react *ReAct, workdir string) *PromptManager {
	return &PromptManager{
		cpm:     aicommon.NewContextProviderManager(),
		react:   react,
		workdir: workdir,
	}
}

// GetGlanceWorkdir caches and returns the directory listing snapshot used by
// workspace-aware prompts.
func (pm *PromptManager) GetGlanceWorkdir(wd string) string {
	pm.glanceWorkdir = filesys.Glance(wd)
	return pm.glanceWorkdir
}

// GetAvailableAIForgeBlueprints returns the forge inventory rendered for the
// base prompt; empty on any manager failure so callers can degrade gracefully.
func (pm *PromptManager) GetAvailableAIForgeBlueprints() string {
	mgr := pm.react.config.GetAIForgeManager()
	if mgr == nil {
		log.Warnf("cannot query any ai-forge manager: nil manager")
		return ""
	}
	forges, err := mgr.Query(pm.react.config.GetContext())
	if err != nil {
		log.Warnf("cannot query any ai-forge manager: %v", err)
		return ""
	}
	result, err := mgr.GenerateAIForgeListForPrompt(forges)
	if err != nil {
		log.Warnf("cannot generate ai-forge list for prompt: %v", err)
		return ""
	}
	return result
}

// currentUserInput returns the originating user query for the active task, or
// "" when no task is running. Centralised so every prompt builder shares one
// retrieval path.
func (pm *PromptManager) currentUserInput() string {
	if task := pm.react.GetCurrentTask(); task != nil {
		return task.GetUserInput()
	}
	return ""
}

// currentLoopInstructionAndExample fetches the persistent instruction and
// output example injected by the active ReActLoop, falling back to the
// supplied defaults when the loop has none.
func (pm *PromptManager) currentLoopInstructionAndExample(
	fallbackInstruction, fallbackExample string,
) (instruction, example string) {
	instruction = fallbackInstruction
	example = fallbackExample
	if loop := pm.react.GetCurrentLoop(); loop != nil {
		if v := loop.GetPersistentInstruction(); v != "" {
			instruction = v
		}
		if v := loop.GetOutputExample(); v != "" {
			example = v
		}
	}
	return instruction, example
}

// currentLoopSchema returns the schema recorded by the most recent main loop
// prompt generation, so sub-role prompts reuse R1's semi-dynamic-2 schema for
// prefix cache alignment.
func (pm *PromptManager) currentLoopSchema() string {
	if loop := pm.react.GetCurrentLoop(); loop != nil {
		return loop.GetLastLoopSchema()
	}
	return ""
}

// toolParamNames extracts and sorts the input parameter names declared on a
// tool's JSON schema, used to render AITAG hints for the parameter generation
// prompt.
func toolParamNames(tool *aitool.Tool) []string {
	var names []string
	if tool.Tool != nil && tool.Tool.InputSchema.Properties != nil {
		tool.Tool.InputSchema.Properties.ForEach(func(name string, _ any) bool {
			names = append(names, name)
			return true
		})
		sort.Strings(names)
	}
	return names
}

// GetBasicPromptInfo renders the legacy base.txt template map. It is the only
// remaining consumer of base.txt and is used by loop-level persistent
// instruction / output-example providers that render caller-specific templates
// against the full environment+tool+timeline context.
func (pm *PromptManager) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	result := make(map[string]any)
	// Minute-granularity timestamp keeps base.txt byte-stable within a minute,
	// preserving prefix cache hits for the surrounding sections.
	result["CurrentTime"] = time.Now().Format("2006-01-02 15:04")
	result["OSArch"] = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	result["WorkingDir"] = pm.workdir
	result["WorkingDirGlance"] = pm.GetGlanceWorkdir(pm.workdir)
	generatedNonce := nonce()
	result["Nonce"] = generatedNonce
	result["DynamicContext"] = pm.DynamicContextWithNonce(generatedNonce)
	result["Language"] = pm.react.config.GetLanguage()

	taskType := "react"
	if forgeName := pm.react.config.GetForgeName(); forgeName != "" {
		taskType = "forge"
		result["ForgeName"] = forgeName
	}
	result["TaskType"] = taskType

	allowPlanAndExec := pm.react.config.GetEnablePlanAndExec() && pm.react.GetCurrentPlanExecutionTask() == nil
	result["AllowPlan"] = allowPlanAndExec
	if allowPlanAndExec {
		result["AIForgeList"] = pm.GetAvailableAIForgeBlueprints()
	}
	result["ShowForgeList"] = pm.react.config.GetShowForgeListInPrompt()
	result["AllowAskForClarification"] = pm.react.config.GetEnableUserInteract()
	result["AllowKnowledgeEnhanceAnswer"] = pm.react.config.GetEnhanceKnowledgeManager() == nil ||
		!pm.react.config.GetDisableEnhanceDirectlyAnswer()
	result["AskForClarificationCurrentTime"] = pm.react.currentUserInteractiveCount
	result["AskForClarificationMaxTimes"] = pm.react.config.GetUserInteractiveLimitedTimes()

	if len(tools) > 0 {
		// caller-supplied subset: render verbatim, no token-budget trimming.
		result["Tools"] = tools
		result["ToolsCount"] = len(tools)
		result["TopToolsCount"] = len(tools)
		result["TopTools"] = tools
		result["HasMoreTools"] = false
		result["MoreToolsCount"] = 0
	} else {
		toolMgr := pm.react.config.GetAiToolManager()
		if toolMgr == nil {
			return "", nil, fmt.Errorf("ai tool manager is nil")
		}
		available, err := toolMgr.GetEnableTools()
		if err != nil {
			return "", nil, err
		}
		available = aicommon.ResolvePromptCandidateTools(pm.react.config, available)
		selection := aicommon.ResolvePromptToolInventory(pm.react.config, available, nil, true)
		result["Tools"] = selection.VisibleTools
		result["ToolsCount"] = len(selection.VisibleTools)
		if len(selection.VisibleTools) > 0 {
			result["TopTools"] = selection.DisplayTools
			result["TopToolsCount"] = len(selection.DisplayTools)
			result["HasMoreTools"] = selection.MoreToolsCount() > 0
			result["MoreToolsCount"] = selection.MoreToolsCount()
		} else {
			result["TopTools"] = []*aitool.Tool{}
			result["TopToolsCount"] = 0
			result["HasMoreTools"] = false
			result["MoreToolsCount"] = 0
		}
	}

	result["Timeline"] = pm.timelineDumpForPrompt()
	return basePrompt, result, nil
}

// preparePromptPrefixMaterials builds the shared five-section prefix
// materials (base + PromptMaterials) used by every Generate* method. The
// caller customises TaskInstruction / OutputExample / Schema / capability
// flags on the returned PromptMaterials before handing it to
// assemblePromptWithDynamicSection.
func (pm *PromptManager) preparePromptPrefixMaterials(
	tools []*aitool.Tool,
	input *reactloops.LoopPromptAssemblyInput,
) (*reactloops.LoopPromptBaseMaterials, *aicommon.PromptMaterials, error) {
	if input == nil {
		return nil, nil, fmt.Errorf("prompt assembly input is nil")
	}
	base, err := pm.GetLoopPromptBaseMaterials(tools, input.Nonce)
	if err != nil {
		return nil, nil, err
	}
	return base, pm.NewPromptMaterials(base, input), nil
}

// assemblePromptWithDynamicSection renders the dynamic tail and joins it with
// the shared prefix sections, producing the final tagged prompt string.
func (pm *PromptManager) assemblePromptWithDynamicSection(
	materials *aicommon.PromptMaterials,
	dynamicTemplateName string,
	dynamicTemplate string,
	dynamicData any,
) (string, error) {
	return aicommon.NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(
		materials,
		dynamicTemplateName,
		dynamicTemplate,
		dynamicData,
		materials.Nonce,
	)
}

// applyLoopInstructionAndExample fills TaskInstruction / OutputExample on the
// prefix materials from the active loop, falling back to the caller-supplied
// defaults. It also attaches the skills context rendered from the loop's
// SkillsContextManager.
func (pm *PromptManager) applyLoopInstructionAndExample(
	materials *aicommon.PromptMaterials,
	fallbackInstruction, fallbackExample string,
) {
	instruction, example := pm.currentLoopInstructionAndExample(fallbackInstruction, fallbackExample)
	materials.TaskInstruction = instruction
	materials.OutputExample = example
	materials.SkillsContext = pm.renderSkillsContextForPrompt()
}

// ToolParamsPromptResult contains the generated prompt and metadata for AITAG
// parsing.
type ToolParamsPromptResult struct {
	Prompt     string
	Nonce      string
	ParamNames []string
}

// GenerateToolParamsPromptWithMeta generates the tool parameter generation
// prompt. It reuses the main loop's schema/instruction/example for R1→R2
// prefix cache alignment; the tool-specific schema lives in the dynamic
// section.
func (pm *PromptManager) GenerateToolParamsPromptWithMeta(tool *aitool.Tool) (*ToolParamsPromptResult, error) {
	nonceString := nonce()
	toolSchema := ""
	if tool.Tool != nil {
		toolSchema = tool.ToJSONSchemaString()
	}
	paramNames := toolParamNames(tool)
	originalQuery := pm.currentUserInput()

	_, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:  nonceString,
		Schema: pm.currentLoopSchema(),
	})
	if err != nil {
		return nil, err
	}
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	pm.applyLoopInstructionAndExample(prefixMaterials, toolParamsInstructionText, toolParamsOutputExampleText)

	dynamicData := pm.buildLoopPromptSectionData(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: originalQuery,
	})
	dynamicData["ToolName"] = tool.Name
	dynamicData["ToolDescription"] = tool.Description
	dynamicData["ToolUsage"] = tool.Usage
	dynamicData["ParamNames"] = paramNames
	dynamicData["OriginalQuery"] = originalQuery
	dynamicData["CurrentIteration"] = pm.react.currentIteration
	dynamicData["MaxIterations"] = int(pm.react.config.GetMaxIterations())
	dynamicData["ToolSchema"] = toolSchema

	prompt, err := pm.assemblePromptWithDynamicSection(
		prefixMaterials, "tool-params-dynamic", toolParamsDynamicTemplate, dynamicData,
	)
	if err != nil {
		return nil, err
	}
	return &ToolParamsPromptResult{
		Prompt:     prompt,
		Nonce:      nonceString,
		ParamNames: paramNames,
	}, nil
}

// GenerateVerificationPrompt generates the verification prompt using the shared
// prefix assembly path. Verification-specific rules and few-shot live in
// semi-dynamic-2; volatile per-iteration data lives in the dynamic tail.
func (pm *PromptManager) GenerateVerificationPrompt(
	originalQuery string, isToolResult bool, payload string, enhanceData ...string,
) (string, string, error) {
	nonceString := nonce()
	_, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:  nonceString,
		Schema: verificationSchemaJSON,
	})
	if err != nil {
		return "", "", err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(verificationInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(verificationOutputExampleText)
	prefixMaterials.SkillsContext = pm.renderSkillsContextForPrompt()

	dynamicData := pm.buildLoopPromptSectionData(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: originalQuery,
	})
	dynamicData["IsToolCall"] = isToolResult
	dynamicData["Payload"] = payload
	dynamicData["TodoSnapshot"] = pm.react.RenderVerificationTodoSnapshot()
	dynamicData["EnhanceData"] = enhanceData
	dynamicData["IterationIndex"] = 0
	dynamicData["MaxIterations"] = 0
	if currentLoop := pm.react.GetCurrentLoop(); currentLoop != nil {
		dynamicData["IterationIndex"] = currentLoop.GetCurrentIterationIndex()
		dynamicData["MaxIterations"] = currentLoop.GetMaxIterations()
	}

	prompt, err := pm.assemblePromptWithDynamicSection(
		prefixMaterials, "verification-dynamic", verificationDynamicTemplate, dynamicData,
	)
	return prompt, nonceString, err
}

// GenerateDirectlyAnswerPrompt generates the directly-answer prompt using the
// shared prefix assembly path.
func (pm *PromptManager) GenerateDirectlyAnswerPrompt(userQuery string, tools []*aitool.Tool) (string, string, error) {
	nonceString := utils.RandStringBytes(4)
	_, prefixMaterials, err := pm.preparePromptPrefixMaterials(tools, &reactloops.LoopPromptAssemblyInput{
		Nonce:  nonceString,
		Schema: getDirectlyAnswer(),
	})
	if err != nil {
		return "", "", err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(directlyAnswerInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(directlyAnswerOutputExampleText)
	prefixMaterials.SkillsContext = pm.renderSkillsContextForPrompt()

	dynamicData := pm.buildLoopPromptSectionData(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
	})
	dynamicData["Language"] = pm.react.config.GetLanguage()

	prompt, err := pm.assemblePromptWithDynamicSection(
		prefixMaterials, "directly-answer-dynamic", directlyAnswerDynamicTemplate, dynamicData,
	)
	return prompt, nonceString, err
}

// GenerateToolReSelectPrompt generates the tool reselection prompt used when a
// previously chosen tool is deemed wrong for the task.
func (pm *PromptManager) GenerateToolReSelectPrompt(
	noUserInteract bool, oldTool *aitool.Tool, toolList []*aitool.Tool,
) (string, error) {
	nonceString := utils.RandStringBytes(4)
	userQuery := pm.currentUserInput()

	_, prefixMaterials, err := pm.preparePromptPrefixMaterials(toolList, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
		Schema:    getReSelectTool(noUserInteract),
	})
	if err != nil {
		return "", err
	}
	prefixMaterials.AllowToolCall = true
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(wrongToolInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(wrongToolOutputExampleText)
	prefixMaterials.SkillsContext = pm.renderSkillsContextForPrompt()

	dynamicData := pm.buildLoopPromptSectionData(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
	})
	if oldTool != nil {
		dynamicData["OldToolName"] = oldTool.Name
		dynamicData["OldToolDescription"] = oldTool.Description
	} else {
		dynamicData["OldToolName"] = ""
		dynamicData["OldToolDescription"] = ""
	}

	return pm.assemblePromptWithDynamicSection(
		prefixMaterials, "wrong-tool-dynamic", wrongToolDynamicTemplate, dynamicData,
	)
}

// GenerateReGenerateToolParamsPromptWithMeta generates a tool parameter
// regeneration prompt (retry after invalid params), reusing the main loop's
// schema/instruction/example for R1→R3 prefix cache alignment.
func (pm *PromptManager) GenerateReGenerateToolParamsPromptWithMeta(
	userQuery string, oldParams aitool.InvokeParams, oldTool *aitool.Tool,
) (*ToolParamsPromptResult, error) {
	nonceString := nonce()
	schemaString := oldTool.ToJSONSchemaString()
	oldParamsDump := oldParams.Dump()
	paramNames := toolParamNames(oldTool)

	_, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:  nonceString,
		Schema: pm.currentLoopSchema(),
	})
	if err != nil {
		return nil, err
	}
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	pm.applyLoopInstructionAndExample(prefixMaterials, toolParamsInstructionText, toolParamsOutputExampleText)

	dynamicData := pm.buildLoopPromptSectionData(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
	})
	dynamicData["ToolName"] = oldTool.Name
	dynamicData["ToolDescription"] = oldTool.Description
	dynamicData["ToolUsage"] = oldTool.Usage
	dynamicData["OldParams"] = oldParamsDump
	dynamicData["ParamNames"] = paramNames
	dynamicData["ToolSchema"] = schemaString

	prompt, err := pm.assemblePromptWithDynamicSection(
		prefixMaterials, "tool-params-dynamic", toolParamsDynamicTemplate, dynamicData,
	)
	if err != nil {
		return nil, err
	}
	return &ToolParamsPromptResult{
		Prompt:     prompt,
		Nonce:      nonceString,
		ParamNames: paramNames,
	}, nil
}

// GenerateChangeAIBlueprintPrompt generates the prompt that asks the model to
// switch to a different AI Forge blueprint.
func (pm *PromptManager) GenerateChangeAIBlueprintPrompt(
	ins *schema.AIForge, forgeList string, oldParams aitool.InvokeParams, extraPrompt string,
) (string, error) {
	nonceString := utils.RandStringBytes(4)
	if utils.IsNil(oldParams) || len(oldParams) <= 0 {
		oldParams = nil
	}
	userQuery := pm.currentUserInput()

	_, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
		Schema:    getChangeAIBlueprintSchema(),
	})
	if err != nil {
		return "", err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = true
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(changeBlueprintInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(changeBlueprintOutputExampleText)
	prefixMaterials.SkillsContext = pm.renderSkillsContextForPrompt()
	// Surface the forge inventory passed by the caller in the frozen-block so the
	// change-blueprint instruction can reference it ("from the inventory above").
	prefixMaterials.ForgeInventory = strings.TrimSpace(forgeList) != ""
	prefixMaterials.AIForgeList = forgeList

	dynamicData := pm.buildLoopPromptSectionData(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
	})
	dynamicData["CurrentBlueprintName"] = ins.ForgeName
	dynamicData["CurrentBlueprintDescription"] = ins.Description
	dynamicData["ExtraPrompt"] = extraPrompt
	dynamicData["Language"] = pm.react.config.GetLanguage()
	if utils.IsNil(oldParams) || len(oldParams) <= 0 {
		dynamicData["OldParams"] = ""
	} else {
		dynamicData["OldParams"] = oldParams.Dump()
	}

	return pm.assemblePromptWithDynamicSection(
		prefixMaterials, "change-blueprint-dynamic", changeBlueprintDynamicTemplate, dynamicData,
	)
}

// GenerateAIBlueprintForgeParamsPromptEx generates a blueprint parameter
// generation (or regeneration) prompt, reusing the main loop's
// schema/instruction/example for R1→R5 prefix cache alignment.
func (pm *PromptManager) GenerateAIBlueprintForgeParamsPromptEx(
	ins *schema.AIForge, blueprintSchema string, oldParams aitool.InvokeParams, extraPrompt string,
) (string, error) {
	nonceString := utils.RandStringBytes(4)
	originalQuery := pm.currentUserInput()

	_, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: originalQuery,
		Schema:    blueprintSchema,
	})
	if err != nil {
		return "", err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = true
	prefixMaterials.HasLoadCapability = false
	pm.applyLoopInstructionAndExample(prefixMaterials, toolParamsInstructionText, toolParamsOutputExampleText)

	dynamicData := pm.buildLoopPromptSectionData(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: originalQuery,
	})
	dynamicData["ToolName"] = ins.ForgeName
	dynamicData["ToolDescription"] = ins.Description
	dynamicData["OldParams"] = ""
	if !utils.IsNil(oldParams) && len(oldParams) > 0 {
		dynamicData["OldParams"] = oldParams.Dump()
	}
	dynamicData["IsBlueprint"] = true
	dynamicData["ExtraPrompt"] = extraPrompt
	dynamicData["CurrentIteration"] = pm.react.currentIteration
	dynamicData["MaxIterations"] = int(pm.react.config.GetMaxIterations())
	return pm.assemblePromptWithDynamicSection(
		prefixMaterials, "tool-params-dynamic", toolParamsDynamicTemplate, dynamicData,
	)
}

// GenerateAIBlueprintForgeParamsPrompt is the zero-extra-prompt convenience
// wrapper around GenerateAIBlueprintForgeParamsPromptEx.
func (pm *PromptManager) GenerateAIBlueprintForgeParamsPrompt(
	ins *schema.AIForge, blueprintSchema string,
) (string, error) {
	return pm.GenerateAIBlueprintForgeParamsPromptEx(ins, blueprintSchema, nil, "")
}

// GenerateRequireConversationTitlePrompt renders the short conversation-title
// utility prompt directly. It has no schema or reusable few-shot block, so the
// shared prefix path would add overhead without cache benefit.
func (pm *PromptManager) GenerateRequireConversationTitlePrompt(timeline string, userInput string) (string, error) {
	return pm.executeTemplate("conversation-title", conversationTitlePrompt, map[string]interface{}{
		"Timeline":     timeline,
		"CurrentInput": userInput,
	})
}

// executeTemplate renders a named text/template. Kept as a method so
// prompt_loop_materials.go can reuse it; prefer aicommon.RenderPromptTemplate
// for new callers.
func (pm *PromptManager) executeTemplate(name, templateContent string, data interface{}) (string, error) {
	return aicommon.RenderPromptTemplate(name, templateContent, data)
}

func (pm *PromptManager) timelineDumpForPrompt() string {
	if pm == nil || pm.react == nil || pm.react.config == nil {
		return ""
	}
	timeline := pm.react.config.GetTimeline()
	if timeline == nil {
		return ""
	}
	return buildTimelineDumpWithMidtermMemory(pm.react, timeline)
}

// DynamicContext returns the concatenation of auto (provider) context and user
// history context without a nonce. Used by tests and legacy diagnostics.
func (pm *PromptManager) DynamicContext() string {
	baseContext := pm.AutoContext()
	historyContext := pm.UserHistoryContext()
	switch {
	case baseContext == "":
		return historyContext
	case historyContext == "":
		return baseContext
	default:
		return baseContext + "\n\n" + historyContext
	}
}

// DynamicContextWithNonce returns the nonce-tagged concatenation of auto
// (provider) context and user history context, used by the live prompt
// builders.
func (pm *PromptManager) DynamicContextWithNonce(nonce string) string {
	baseContext := pm.AutoContextWithNonce(nonce)
	historyContext := pm.UserHistoryContextWithNonce(nonce)
	switch {
	case strings.TrimSpace(baseContext) == "":
		return historyContext
	case strings.TrimSpace(historyContext) == "":
		return baseContext
	default:
		return baseContext + "\n\n" + historyContext
	}
}

// renderSkillsContextForPrompt mirrors the main ReAct loop skills block so
// tool parameter generation can reuse loaded SKILL.md guidance.
func (pm *PromptManager) renderSkillsContextForPrompt() string {
	if pm == nil || pm.react == nil {
		return ""
	}
	currentLoop := pm.react.GetCurrentLoop()
	if currentLoop == nil {
		return ""
	}
	mgr := currentLoop.GetSkillsContextManager()
	if mgr == nil {
		return ""
	}
	return mgr.RenderStable()
}

// GenerateIntervalReviewPromptWithContext generates a dedicated bounded prompt
// for speed-priority progress review of one running tool call.
//
// Lightweight context control:
//   - does not inherit the main loop's high-static / frozen Timeline / promoted state
//   - reads only the most recent 2048 tokens of prompt-visible Timeline
//   - review rules / schema / example are pinned in semi-dynamic-2
//   - each dynamic field has an independent token budget; the whole prompt has
//     a 9000-token hard cap
func (pm *PromptManager) GenerateIntervalReviewPromptWithContext(
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	startTime time.Time,
	reviewCount int,
	callExpectations string,
) (string, error) {
	nonceString := nonce()
	const (
		intervalReviewMaxPromptTokens        = 9000
		intervalReviewTimelineTokens         = 2048
		intervalReviewUserQueryTokens        = 1024
		intervalReviewTaskGoalTokens         = 256
		intervalReviewToolDescriptionTokens  = 512
		intervalReviewToolNameTokens         = 128
		intervalReviewToolParamsTokens       = 1536
		intervalReviewStdoutTokens           = 1024
		intervalReviewStderrTokens           = 512
		intervalReviewCallExpectationTokens  = 512
		intervalReviewExtraInstructionTokens = 512
	)

	userQuery := ""
	taskGoal := ""
	if task := pm.react.GetCurrentTask(); task != nil {
		userQuery = aicommon.ShrinkTextBlockByTokens(task.GetUserInput(), intervalReviewUserQueryTokens)
		taskGoal = aicommon.ShrinkTextBlockByTokens(task.GetName(), intervalReviewTaskGoalTokens)
	}

	dynamicData := map[string]any{
		"Nonce":     nonceString,
		"UserQuery": userQuery,
	}
	dynamicData["ToolName"] = aicommon.ShrinkStringByTokens(tool.Name, intervalReviewToolNameTokens)
	dynamicData["ToolDescription"] = aicommon.ShrinkTextBlockByTokens(tool.Description, intervalReviewToolDescriptionTokens)
	dynamicData["ToolParams"] = aicommon.ShrinkTextBlockByTokens(params.Dump(), intervalReviewToolParamsTokens)
	dynamicData["CurrentTime"] = time.Now().Format("2006-01-02 15:04:05")
	dynamicData["ReviewCount"] = reviewCount
	dynamicData["StdoutSnapshot"] = aicommon.ShrinkTextBlockByTokens(string(stdoutSnapshot), intervalReviewStdoutTokens)
	dynamicData["StderrSnapshot"] = aicommon.ShrinkTextBlockByTokens(string(stderrSnapshot), intervalReviewStderrTokens)
	dynamicData["CallExpectations"] = aicommon.ShrinkTextBlockByTokens(callExpectations, intervalReviewCallExpectationTokens)
	dynamicData["ExtraPrompt"] = aicommon.ShrinkTextBlockByTokens(
		strings.TrimSpace(pm.react.config.GetConfigString(aicommon.ConfigKeyToolCallIntervalReviewExtraPrompt)),
		intervalReviewExtraInstructionTokens,
	)
	dynamicData["TaskGoal"] = taskGoal

	if !startTime.IsZero() {
		elapsed := time.Since(startTime)
		dynamicData["StartTime"] = startTime.Format("2006-01-02 15:04:05")
		dynamicData["ElapsedDuration"] = formatDuration(elapsed)
	} else {
		dynamicData["ElapsedDuration"] = "unknown"
		dynamicData["StartTime"] = "unknown"
	}

	dynamic, err := aicommon.RenderPromptTemplate("interval-review-dynamic", intervalReviewDynamicTemplate, dynamicData)
	if err != nil {
		return "", err
	}
	semiDynamic2, err := aicommon.RenderPromptTemplate(
		"interval-review-semi-dynamic-2",
		aicommon.SharedTaskInstructionSchemaExampleTemplate,
		(&aicommon.PromptMaterials{
			TaskInstruction: strings.TrimSpace(intervalReviewInstructionText),
			Schema:          strings.TrimSpace(intervalReviewSchemaJSON),
			OutputExample:   strings.TrimSpace(intervalReviewOutputExampleText),
		}).SemiDynamic2Data(),
	)
	if err != nil {
		return "", err
	}
	recentTimeline := ""
	if pm.react != nil && pm.react.config != nil && pm.react.config.GetTimeline() != nil {
		recentTimeline = pm.react.config.GetTimeline().DumpRecentForPrompt(intervalReviewTimelineTokens)
	}
	if recentTimeline == "" {
		recentTimeline = "<|TIMELINE_RECENT|>\n(no recent Timeline items)\n<|TIMELINE_RECENT_END|>"
	}
	prompt := aicommon.BuildTaggedPromptSections(
		"You are performing a bounded progress review for one running tool call.",
		"",
		"",
		semiDynamic2,
		recentTimeline,
		dynamic,
		nonceString,
	)
	if tokens := aicommon.MeasureTokens(prompt); tokens > intervalReviewMaxPromptTokens {
		return "", fmt.Errorf("interval review prompt exceeds %d-token hard limit: %d", intervalReviewMaxPromptTokens, tokens)
	}
	return prompt, nil
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1f seconds", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%d min %d sec", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%d hour %d min", int(d.Hours()), int(d.Minutes())%60)
	}
}
