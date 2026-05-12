package aireact

import (
	"bytes"
	_ "embed"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"text/template"
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

//go:embed prompts/review/ai-review-tool-call_instruction.txt
var aiReviewInstructionText string

//go:embed prompts/review/ai-review-tool-call_output_example.txt
var aiReviewOutputExampleText string

//go:embed prompts/review/ai-review-tool-call_dynamic.txt
var aiReviewDynamicTemplate string

//go:embed prompts/review/ai-review-tool-call.json
var aiReviewSchemaJSON string

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

//go:embed prompts/tool/wrong-params_instruction.txt
var wrongParamsInstructionText string

//go:embed prompts/tool/wrong-params_output_example.txt
var wrongParamsOutputExampleText string

//go:embed prompts/tool/wrong-params_dynamic.txt
var wrongParamsDynamicTemplate string

//go:embed prompts/tool/interval-review_instruction.txt
var intervalReviewInstructionText string

//go:embed prompts/tool/interval-review_output_example.txt
var intervalReviewOutputExampleText string

//go:embed prompts/tool/interval-review_dynamic.txt
var intervalReviewDynamicTemplate string

//go:embed prompts/tool/interval-review.json
var intervalReviewSchemaJSON string

//go:embed prompts/tool-params/blueprint-params_instruction.txt
var blueprintParamsInstructionText string

//go:embed prompts/tool-params/blueprint-params_output_example.txt
var blueprintParamsOutputExampleText string

//go:embed prompts/tool-params/blueprint-params_dynamic.txt
var blueprintParamsDynamicTemplate string

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

// PromptManager manages ReAct prompt templates
type PromptManager struct {
	cpm           *aicommon.ContextProviderManager
	workdir       string
	glanceWorkdir string
	react         *ReAct
}

// NewPromptManager creates a new prompt manager
func NewPromptManager(react *ReAct, workdir string) *PromptManager {
	return &PromptManager{
		cpm:           aicommon.NewContextProviderManager(),
		workdir:       workdir,
		glanceWorkdir: "",
		react:         react,
	}
}

// LoopPromptData contains data for the main loop prompt template
type LoopPromptData struct {
	AllowAskForClarification       bool
	AllowPlan                      bool
	AllowKnowledgeEnhanceAnswer    bool
	AllowWriteYaklangCode          bool
	AskForClarificationCurrentTime int64
	AskForClarificationMaxTimes    int64

	CurrentTime      string
	OSArch           string
	WorkingDir       string
	WorkingDirGlance string
	AIForgeList      string
	ShowForgeList    bool
	Tools            []*aitool.Tool
	ToolsCount       int
	TopTools         []*aitool.Tool
	TopToolsCount    int
	HasMoreTools     bool
	Timeline         string
	UserQuery        string
	Nonce            string
	Language         string
	Schema           string
	DynamicContext   string
	TaskType         string
	ForgeName        string
}

// ToolParamsPromptData contains data for tool parameter generation prompt
type ToolParamsPromptData struct {
	ToolName         string
	ToolDescription  string
	ToolUsage        string // Usage instructions disclosed at param generation stage (2-phase disclosure)
	ToolSchema       string
	OriginalQuery    string
	CurrentIteration int
	MaxIterations    int
	Timeline         string
	DynamicContext   string
	Nonce            string   // Nonce for AITAG format
	ParamNames       []string // List of parameter names for AITAG hints
}

// YaklangCodeActionLoopPromptData contains data for Yaklang code generation action loop prompt
type YaklangCodeActionLoopPromptData struct {
	CurrentTime               string
	OSArch                    string
	WorkingDir                string
	WorkingDirGlance          string
	Timeline                  string
	Nonce                     string
	UserQuery                 string
	CurrentCode               string
	CurrentCodeWithLineNumber string
	IterationCount            int
	ErrorMessages             string
	Language                  string
	DynamicContext            string
	Schema                    string
	Tools                     []*aitool.Tool
	ToolsCount                int
	TopTools                  []*aitool.Tool
	TopToolsCount             int
	HasMoreTools              bool
}

func (pm *PromptManager) GetGlanceWorkdir(wd string) string {
	pm.glanceWorkdir = filesys.Glance(wd)
	return pm.glanceWorkdir
}

func (pm *PromptManager) GetAvailableAIForgeBlueprints() string {
	// use getter and nil-check for safety
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

func (pm *PromptManager) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	result := make(map[string]any)
	// P1-C1: CurrentTime 改为分钟粒度, 让 base.txt 渲染产物在同一分钟内 byte-stable.
	// 历史使用 "2006-01-02 15:04:05" 秒级粒度会让 ReActLoop / aimemory 路径下的
	// .Background 段每秒变化, 直接打散 PROMPT_SECTION_semi-dynamic 段命中率.
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

	// Use getters instead of direct field access

	allowPlanAndExec := pm.react.config.GetEnablePlanAndExec() && pm.react.GetCurrentPlanExecutionTask() == nil

	result["AllowPlan"] = allowPlanAndExec
	if allowPlanAndExec {
		result["AIForgeList"] = pm.GetAvailableAIForgeBlueprints()
	}
	// ShowForgeList controls whether forge list is rendered in base prompt
	// Default false: forges are discoverable via search_capabilities instead of being listed in prompt
	result["ShowForgeList"] = pm.react.config.GetShowForgeListInPrompt()
	result["AllowAskForClarification"] = pm.react.config.GetEnableUserInteract()
	result["AllowKnowledgeEnhanceAnswer"] = pm.react.config.GetEnhanceKnowledgeManager() == nil || !pm.react.config.GetDisableEnhanceDirectlyAnswer()
	result["AskForClarificationCurrentTime"] = pm.react.currentUserInteractiveCount
	result["AskForClarificationMaxTimes"] = pm.react.config.GetUserInteractiveLimitedTimes()
	if len(tools) > 0 {
		result["Tools"] = tools
		result["ToolsCount"] = len(tools)
		result["TopToolsCount"] = len(tools)
		result["TopTools"] = tools
		result["HasMoreTools"] = false
	} else {
		var err error
		// use getter for ai tool manager and handle nil
		toolMgr := pm.react.config.GetAiToolManager()
		if toolMgr == nil {
			return "", nil, fmt.Errorf("ai tool manager is nil")
		}
		tools, err = toolMgr.GetEnableTools()
		if err != nil {
			return "", nil, err
		}
		result["Tools"] = tools
		result["ToolsCount"] = len(tools)
		result["TopToolsCount"] = pm.react.config.GetTopToolsCount()
		// Get prioritized tools
		if len(tools) > 0 {
			topTools := pm.react.getPrioritizedTools(tools, pm.react.config.GetTopToolsCount())
			result["TopTools"] = topTools
			result["HasMoreTools"] = len(tools) > len(topTools)
		} else {
			result["TopTools"] = []*aitool.Tool{}
			result["HasMoreTools"] = false
		}
	}

	// use timeline getter
	result["Timeline"] = pm.timelineDumpForPrompt()
	return basePrompt, result, nil
}

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

// ToolParamsPromptResult contains the generated prompt and metadata for AITAG parsing
type ToolParamsPromptResult struct {
	Prompt     string
	Nonce      string
	ParamNames []string
	Identifier string // destination identifier extracted from AI response, e.g. "query_large_file", "find_process"
}

// GenerateToolParamsPrompt generates tool parameter generation prompt using template
func (pm *PromptManager) GenerateToolParamsPrompt(tool *aitool.Tool) (string, error) {
	result, err := pm.GenerateToolParamsPromptWithMeta(tool)
	if err != nil {
		return "", err
	}
	return result.Prompt, nil
}

// GenerateToolParamsPromptWithMeta generates tool parameter generation prompt with metadata for AITAG parsing
func (pm *PromptManager) GenerateToolParamsPromptWithMeta(tool *aitool.Tool) (*ToolParamsPromptResult, error) {
	generatedNonce := nonce()
	data := &ToolParamsPromptData{
		ToolName:        tool.Name,
		ToolDescription: tool.Description,
		ToolUsage:       tool.Usage,
		DynamicContext:  pm.DynamicContextWithNonce(generatedNonce),
		Nonce:           generatedNonce, // Generate nonce for AITAG format
	}

	// Set tool schema if available
	if tool.Tool != nil {
		data.ToolSchema = tool.ToJSONSchemaString()
		// Extract parameter names for AITAG hints
		if tool.Tool.InputSchema.Properties != nil {
			tool.Tool.InputSchema.Properties.ForEach(func(name string, _ any) bool {
				data.ParamNames = append(data.ParamNames, name)
				return true
			})
			sort.Strings(data.ParamNames)
		}
	}

	// Extract context data from memory without lock (assume caller already holds lock)
	if pm.react.config.GetTimeline() != nil {
		if task := pm.react.GetCurrentTask(); task != nil {
			data.OriginalQuery = task.GetUserInput()
		}
		data.Timeline = pm.timelineDumpForPrompt()
	}
	data.CurrentIteration = pm.react.currentIteration
	data.MaxIterations = int(pm.react.config.GetMaxIterations())

	_, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:  generatedNonce,
		Schema: strings.TrimSpace(data.ToolSchema),
	})
	if err != nil {
		return nil, err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(toolParamsInstructionText)
	prefixMaterials.OutputExample = ""
	prefixMaterials.ToolInventory = false
	prefixMaterials.ToolsCount = 0
	prefixMaterials.TopToolsCount = 0
	prefixMaterials.TopTools = nil
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = false
	prefixMaterials.AIForgeList = ""
	prefixMaterials.SkillsContext = ""

	prompt, err := pm.assemblePromptWithDynamicSection(
		prefixMaterials,
		"tool-params-dynamic",
		toolParamsDynamicTemplate,
		data,
	)
	if err != nil {
		return nil, err
	}

	return &ToolParamsPromptResult{
		Prompt:     prompt,
		Nonce:      generatedNonce,
		ParamNames: data.ParamNames,
	}, nil
}

// GenerateVerificationPrompt generates verification prompt using shared prompt
// prefix assembly.
//
// aicache 命中率优化:
//   - 复用 preparePromptPrefixMaterials + assemblePromptWithDynamicSection,
//     与 directly-answer / tool-params 走同一套前缀拼装路径
//   - verification 专属规则与 few-shot 落在 semi-dynamic-2
//     (TaskInstruction + Schema + OutputExample)
//   - OriginalQuery / INPUT / TODO 快照 / 迭代上下文 / EnhanceData 留在 dynamic
//     尾段, 避免污染上游 prefix cache
//
// 关键词: GenerateVerificationPrompt, preparePromptPrefixMaterials,
//
//	assemblePromptWithDynamicSection, verification semi-dynamic-2
func (pm *PromptManager) GenerateVerificationPrompt(originalQuery string, isToolResult bool, payload string, enhanceData ...string) (string, string, error) {
	nonceString := nonce()
	base, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
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
	prefixMaterials.ToolInventory = false
	prefixMaterials.ToolsCount = 0
	prefixMaterials.TopToolsCount = 0
	prefixMaterials.TopTools = nil
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = false
	prefixMaterials.AIForgeList = ""
	prefixMaterials.SkillsContext = ""
	prefixMaterials.RecentToolsCache = ""

	dynamicData := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{
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
		prefixMaterials,
		"verification-dynamic",
		verificationDynamicTemplate,
		dynamicData,
	)
	return prompt, nonceString, err
}

// GenerateAIReviewPrompt generates AI tool call review prompt using shared prompt
// prefix assembly.
//
// aicache 命中率优化:
//   - 复用 preparePromptPrefixMaterials + assemblePromptWithDynamicSection
//   - 风险评估规则 / schema / 示例输出下沉到 semi-dynamic-2
//   - 用户 query、待审核实体与语言偏好留在 dynamic，timeline/workspace 复用公共前缀
//
// 关键词: GenerateAIReviewPrompt, ai-review, preparePromptPrefixMaterials,
//
//	assemblePromptWithDynamicSection
func (pm *PromptManager) GenerateAIReviewPrompt(userQuery, toolOrTitle, params string) (string, error) {
	nonceString := nonce()
	base, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
		Schema:    aiReviewSchemaJSON,
	})
	if err != nil {
		return "", err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(aiReviewInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(aiReviewOutputExampleText)
	prefixMaterials.ToolInventory = false
	prefixMaterials.ToolsCount = 0
	prefixMaterials.TopToolsCount = 0
	prefixMaterials.TopTools = nil
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = false
	prefixMaterials.AIForgeList = ""
	prefixMaterials.SkillsContext = ""
	prefixMaterials.RecentToolsCache = ""

	dynamicData := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
	})
	dynamicData["Title"] = toolOrTitle
	dynamicData["Details"] = params
	dynamicData["Language"] = pm.react.config.GetLanguage()

	return pm.assemblePromptWithDynamicSection(
		prefixMaterials,
		"ai-review-dynamic",
		aiReviewDynamicTemplate,
		dynamicData,
	)
}

// GenerateDirectlyAnswerPrompt generates directly answer prompt using template
func (pm *PromptManager) GenerateDirectlyAnswerPrompt(userQuery string, tools []*aitool.Tool) (string, string, error) {
	var directlyAnswerSchema = getDirectlyAnswer()

	nonceString := utils.RandStringBytes(4)
	base, prefixMaterials, err := pm.preparePromptPrefixMaterials(tools, &reactloops.LoopPromptAssemblyInput{
		Nonce:  nonceString,
		Schema: directlyAnswerSchema,
	})
	if err != nil {
		return "", "", err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(directlyAnswerInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(directlyAnswerOutputExampleText)
	prefixMaterials.ToolInventory = false
	prefixMaterials.ToolsCount = 0
	prefixMaterials.TopToolsCount = 0
	prefixMaterials.TopTools = nil
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = false
	prefixMaterials.AIForgeList = ""
	prefixMaterials.SkillsContext = ""

	dynamicData := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
	})
	dynamicData["Language"] = pm.react.config.GetLanguage()

	result, err := pm.assemblePromptWithDynamicSection(
		prefixMaterials,
		"directly-answer-dynamic",
		directlyAnswerDynamicTemplate,
		dynamicData,
	)
	return result, nonceString, err
}

// GenerateToolReSelectPrompt generates tool reselection prompt using template
func (pm *PromptManager) GenerateToolReSelectPrompt(noUserInteract bool, oldTool *aitool.Tool, toolList []*aitool.Tool) (string, error) {
	nonceString := utils.RandStringBytes(4)
	userQuery := ""
	if r := pm.react.GetCurrentTask(); r != nil {
		userQuery = r.GetUserInput()
	}

	base, prefixMaterials, err := pm.preparePromptPrefixMaterials(toolList, &reactloops.LoopPromptAssemblyInput{
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
	prefixMaterials.ToolInventory = len(toolList) > 0
	prefixMaterials.ToolsCount = len(toolList)
	prefixMaterials.TopToolsCount = len(toolList)
	prefixMaterials.TopTools = append([]*aitool.Tool{}, toolList...)
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = false
	prefixMaterials.AIForgeList = ""
	prefixMaterials.SkillsContext = ""

	dynamicData := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{
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
		prefixMaterials,
		"wrong-tool-dynamic",
		wrongToolDynamicTemplate,
		dynamicData,
	)
}

// GenerateReGenerateToolParamsPrompt generates tool parameter regeneration prompt using template
func (pm *PromptManager) GenerateReGenerateToolParamsPrompt(userQuery string, oldParams aitool.InvokeParams, oldTool *aitool.Tool) (string, error) {
	result, err := pm.GenerateReGenerateToolParamsPromptWithMeta(userQuery, oldParams, oldTool)
	if err != nil {
		return "", err
	}
	return result.Prompt, nil
}

// GenerateReGenerateToolParamsPromptWithMeta generates tool parameter regeneration prompt with AITAG metadata
func (pm *PromptManager) GenerateReGenerateToolParamsPromptWithMeta(userQuery string, oldParams aitool.InvokeParams, oldTool *aitool.Tool) (*ToolParamsPromptResult, error) {
	generatedNonce := nonce()
	schemaString := oldTool.ToJSONSchemaString()
	oldParamsDump := oldParams.Dump()
	paramNames := []string{}

	// Extract parameter names for AITAG hints
	if oldTool.Tool != nil && oldTool.Tool.InputSchema.Properties != nil {
		oldTool.Tool.InputSchema.Properties.ForEach(func(name string, _ any) bool {
			paramNames = append(paramNames, name)
			return true
		})
		sort.Strings(paramNames)
	}

	base, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:  generatedNonce,
		Schema: schemaString,
	})
	if err != nil {
		return nil, err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(wrongParamsInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(wrongParamsOutputExampleText)
	prefixMaterials.ToolInventory = false
	prefixMaterials.ToolsCount = 0
	prefixMaterials.TopToolsCount = 0
	prefixMaterials.TopTools = nil
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = false
	prefixMaterials.AIForgeList = ""
	prefixMaterials.SkillsContext = ""

	dynamicData := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{
		Nonce:     generatedNonce,
		UserQuery: userQuery,
	})
	dynamicData["ToolName"] = oldTool.Name
	dynamicData["ToolDescription"] = oldTool.Description
	dynamicData["ToolUsage"] = oldTool.Usage
	dynamicData["OldParams"] = oldParamsDump
	dynamicData["ParamNames"] = paramNames

	prompt, err := pm.assemblePromptWithDynamicSection(
		prefixMaterials,
		"wrong-params-dynamic",
		wrongParamsDynamicTemplate,
		dynamicData,
	)
	if err != nil {
		return nil, err
	}

	return &ToolParamsPromptResult{
		Prompt:     prompt,
		Nonce:      generatedNonce,
		ParamNames: paramNames,
	}, nil
}

func (pm *PromptManager) GenerateChangeAIBlueprintPrompt(
	ins *schema.AIForge,
	forgeList string,
	oldParams aitool.InvokeParams,
	extraPrompt string,
) (string, error) {
	nonceString := utils.RandStringBytes(4)
	if utils.IsNil(oldParams) || len(oldParams) <= 0 {
		oldParams = nil
	}

	userQuery := ""
	if pm.react.config.GetTimeline() != nil {
		if task := pm.react.GetCurrentTask(); task != nil {
			userQuery = task.GetUserInput()
		}
	}

	base, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
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
	prefixMaterials.ToolInventory = false
	prefixMaterials.ToolsCount = 0
	prefixMaterials.TopToolsCount = 0
	prefixMaterials.TopTools = nil
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = strings.TrimSpace(forgeList) != ""
	prefixMaterials.AIForgeList = forgeList
	prefixMaterials.SkillsContext = ""

	dynamicData := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{
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
		prefixMaterials,
		"change-blueprint-dynamic",
		changeBlueprintDynamicTemplate,
		dynamicData,
	)
}

func (pm *PromptManager) GenerateAIBlueprintForgeParamsPromptEx(
	ins *schema.AIForge,
	blueprintSchema string,
	oldParams aitool.InvokeParams,
	extraPrompt string,
) (string, error) {
	nonceString := utils.RandStringBytes(4)
	originalQuery := ""
	if utils.IsNil(oldParams) || len(oldParams) <= 0 {
		oldParams = nil
	} else {
		// keep oldParams for dynamic rendering below
	}

	// Extract context data from memory without lock (assume caller already holds lock)
	if pm.react.config.GetTimeline() != nil {
		if task := pm.react.GetCurrentTask(); task != nil {
			originalQuery = task.GetUserInput()
		}
	}

	base, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
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
	prefixMaterials.TaskInstruction = strings.TrimSpace(blueprintParamsInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(blueprintParamsOutputExampleText)
	prefixMaterials.ToolInventory = false
	prefixMaterials.ToolsCount = 0
	prefixMaterials.TopToolsCount = 0
	prefixMaterials.TopTools = nil
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = false
	prefixMaterials.AIForgeList = ""
	prefixMaterials.SkillsContext = ""

	dynamicData := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: originalQuery,
	})
	dynamicData["BlueprintName"] = ins.ForgeName
	dynamicData["BlueprintDescription"] = ins.Description
	dynamicData["OldParams"] = ""
	if !utils.IsNil(oldParams) && len(oldParams) > 0 {
		dynamicData["OldParams"] = oldParams.Dump()
	}
	dynamicData["ExtraPrompt"] = extraPrompt
	dynamicData["CurrentIteration"] = pm.react.currentIteration
	dynamicData["MaxIterations"] = int(pm.react.config.GetMaxIterations())

	return pm.assemblePromptWithDynamicSection(
		prefixMaterials,
		"blueprint-params-dynamic",
		blueprintParamsDynamicTemplate,
		dynamicData,
	)
}

// GenerateAIBlueprintForgeParamsPrompt generates AI blueprint forge parameter generation prompt using template
func (pm *PromptManager) GenerateAIBlueprintForgeParamsPrompt(ins *schema.AIForge, blueprintSchema string) (string, error) {
	return pm.GenerateAIBlueprintForgeParamsPromptEx(ins, blueprintSchema, nil, "")
}

// GenerateRequireConversationTitlePrompt intentionally keeps direct template rendering.
// This utility prompt is short and almost entirely driven by volatile timeline/current-input
// content, without a schema or reusable few-shot block, so the shared prefix path would add
// section overhead without meaningful prefix-cache benefit.
func (pm *PromptManager) GenerateRequireConversationTitlePrompt(timeline string, userInput string) (string, error) {
	data := map[string]interface{}{
		"Timeline":     timeline,
		"CurrentInput": userInput,
	}
	return pm.executeTemplate("conversation-title", conversationTitlePrompt, data)
}

// executeTemplate executes a template with the given data
func (pm *PromptManager) executeTemplate(name, templateContent string, data interface{}) (string, error) {
	tmpl, err := template.New(name).Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("error parsing %s template: %w", name, err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("error executing %s template: %w", name, err)
	}

	return buf.String(), nil
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

func (pm *PromptManager) DynamicContext() string {
	baseContext := pm.AutoContext()
	historyContext := pm.UserHistoryContext()
	if baseContext == "" {
		return historyContext
	}
	if historyContext == "" {
		return baseContext
	}
	return baseContext + "\n\n" + historyContext
}

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

// GenerateIntervalReviewPrompt generates interval review prompt for long-running tool execution
func (pm *PromptManager) GenerateIntervalReviewPrompt(
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
) (string, error) {
	return pm.GenerateIntervalReviewPromptWithContext(tool, params, stdoutSnapshot, stderrSnapshot, time.Time{}, 0, "")
}

// GenerateIntervalReviewPromptWithContext generates interval review prompt with shared prompt
// prefix assembly.
//
// aicache 命中率优化:
//   - 复用 preparePromptPrefixMaterials + assemblePromptWithDynamicSection
//   - 审核规则 / schema / valid output example 固定在 semi-dynamic-2
//   - 当前时间、运行时长、stdout/stderr 快照、额外提示等高抖动字段保留在 dynamic
//
// 关键词: GenerateIntervalReviewPromptWithContext, interval-review,
//
//	preparePromptPrefixMaterials, assemblePromptWithDynamicSection
func (pm *PromptManager) GenerateIntervalReviewPromptWithContext(
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	startTime time.Time,
	reviewCount int,
	callExpectations string,
) (string, error) {
	nonceString := nonce()
	base, prefixMaterials, err := pm.preparePromptPrefixMaterials(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:  nonceString,
		Schema: intervalReviewSchemaJSON,
	})
	if err != nil {
		return "", err
	}
	prefixMaterials.AllowToolCall = false
	prefixMaterials.AllowPlanAndExec = false
	prefixMaterials.HasLoadCapability = false
	prefixMaterials.TaskInstruction = strings.TrimSpace(intervalReviewInstructionText)
	prefixMaterials.OutputExample = strings.TrimSpace(intervalReviewOutputExampleText)
	prefixMaterials.ToolInventory = false
	prefixMaterials.ToolsCount = 0
	prefixMaterials.TopToolsCount = 0
	prefixMaterials.TopTools = nil
	prefixMaterials.HasMoreTools = false
	prefixMaterials.ForgeInventory = false
	prefixMaterials.AIForgeList = ""
	prefixMaterials.SkillsContext = ""
	prefixMaterials.RecentToolsCache = ""

	userQuery := ""
	taskGoal := ""
	if task := pm.react.GetCurrentTask(); task != nil {
		userQuery = task.GetUserInput()
		taskGoal = task.GetName()
	}

	dynamicData := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{
		Nonce:     nonceString,
		UserQuery: userQuery,
	})
	dynamicData["ToolName"] = tool.Name
	dynamicData["ToolDescription"] = tool.Description
	dynamicData["ToolParams"] = params.Dump()
	dynamicData["CurrentTime"] = time.Now().Format("2006-01-02 15:04:05")
	dynamicData["ReviewCount"] = reviewCount
	dynamicData["StdoutSnapshot"] = utils.ShrinkString(string(stdoutSnapshot), 3000)
	dynamicData["StderrSnapshot"] = utils.ShrinkString(string(stderrSnapshot), 1500)
	dynamicData["CallExpectations"] = callExpectations
	dynamicData["ExtraPrompt"] = strings.TrimSpace(pm.react.config.GetConfigString(aicommon.ConfigKeyToolCallIntervalReviewExtraPrompt))
	dynamicData["TaskGoal"] = taskGoal

	if !startTime.IsZero() {
		elapsed := time.Since(startTime)
		dynamicData["StartTime"] = startTime.Format("2006-01-02 15:04:05")
		dynamicData["ElapsedDuration"] = formatDuration(elapsed)
	} else {
		dynamicData["ElapsedDuration"] = "unknown"
		dynamicData["StartTime"] = "unknown"
	}

	return pm.assemblePromptWithDynamicSection(
		prefixMaterials,
		"interval-review-dynamic",
		intervalReviewDynamicTemplate,
		dynamicData,
	)
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1f seconds", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%d min %d sec", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%d hour %d min", hours, minutes)
}
