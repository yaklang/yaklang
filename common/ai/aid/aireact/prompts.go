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

//go:embed prompts/verification/verification.txt
var verificationPromptTemplate string

//go:embed prompts/verification/verification.json
var verificationSchemaJSON string

//go:embed prompts/review/ai-review-tool-call.txt
var aiReviewPromptTemplate string

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

//go:embed prompts/tool/interval-review.txt
var intervalReviewPromptTemplate string

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

// VerificationPromptData contains data for verification prompt
//
// 关键词: VerificationPromptData, aicache 4 段切分, TimelineFrozen, TimelineOpen
//
// Timeline 字段保留以保证向后兼容; 模板内部已切换到使用 TimelineFrozen +
// TimelineOpen 两个字段, 让 verification prompt 也按 5 段稳定性分层 (high-static
// / semi-dynamic / frozen-block / timeline-open / dynamic) 走 aicache splitter,
// 与 React 主 loop 的 buildTaggedPromptSections 输出对齐。
type VerificationPromptData struct {
	Nonce          string
	OriginalQuery  string
	IsToolCall     bool
	Payload        string
	Timeline       string // 兼容字段: frozen + open 拼接, 老观测路径用
	TimelineFrozen string // 渐稳定前缀, 进 AI_CACHE_FROZEN 块
	TimelineOpen   string // 末段 + midterm, 进 PROMPT_SECTION_timeline-open
	TodoSnapshot   string
	Language       string
	Schema         string
	DynamicContext string
	EnhanceData    []string
	IterationIndex int
	MaxIterations  int
}

// AIReviewPromptData contains data for AI tool call review prompt
type AIReviewPromptData struct {
	CurrentTime      string
	OSArch           string
	WorkingDir       string
	WorkingDirGlance string
	Timeline         string
	Nonce            string
	UserQuery        string
	Title            string
	Details          string
	Language         string
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
) (*reactloops.LoopPromptBaseMaterials, *reactloops.PromptPrefixMaterials, error) {
	if input == nil {
		return nil, nil, fmt.Errorf("prompt assembly input is nil")
	}

	base, err := pm.GetLoopPromptBaseMaterials(tools, input.Nonce)
	if err != nil {
		return nil, nil, err
	}
	return base, pm.NewPromptPrefixMaterials(base, input), nil
}

func (pm *PromptManager) assemblePromptWithDynamicSection(
	materials *reactloops.PromptPrefixMaterials,
	dynamicTemplateName string,
	dynamicTemplate string,
	dynamicData any,
) (string, error) {
	prefix, err := pm.AssemblePromptPrefix(materials)
	if err != nil {
		return "", err
	}
	dynamicSection, err := pm.executeTemplate(dynamicTemplateName, dynamicTemplate, dynamicData)
	if err != nil {
		return "", err
	}
	return buildTaggedPromptSections(prefix.HighStatic, prefix.FrozenBlock, prefix.SemiDynamic, prefix.TimelineOpen, dynamicSection, materials.Nonce), nil
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

// GenerateVerificationPrompt generates verification prompt using template.
//
// aicache 命中率优化 (P0-A1):
//   - 模板已重构为 5 段稳定性分层结构 (high-static / semi-dynamic /
//     frozen-block / timeline-open / dynamic)
//   - Timeline 拆成 TimelineFrozen + TimelineOpen 两段, frozen 前缀与
//     最末 interval 分别落到不同的 PROMPT_SECTION 包装中, 让 splitter
//     能按 chunk 分别命中缓存
//
// 关键词: GenerateVerificationPrompt, aicache 4 段切分, TimelineFrozen, TimelineOpen
func (pm *PromptManager) GenerateVerificationPrompt(originalQuery string, isToolResult bool, payload string, enhanceData ...string) (string, string, error) {
	nonce := nonce()
	data := &VerificationPromptData{
		Nonce:          nonce,
		OriginalQuery:  originalQuery,
		IsToolCall:     isToolResult,
		Payload:        payload,
		TodoSnapshot:   pm.react.RenderVerificationTodoSnapshot(),
		Language:       pm.react.config.GetLanguage(),
		Schema:         verificationSchemaJSON,
		DynamicContext: pm.DynamicContextWithNonce(nonce),
		EnhanceData:    enhanceData,
	}

	if currentLoop := pm.react.GetCurrentLoop(); currentLoop != nil {
		data.IterationIndex = currentLoop.GetCurrentIterationIndex()
		data.MaxIterations = currentLoop.GetMaxIterations()
	}

	// 同时填充 frozen / open 两段 timeline 以及 legacy Timeline 字段。
	// 模板内部只读 frozen + open; Timeline 字段保留兼容老调用站点。
	// 关键词: timeline frozen/open 双段填充, aicache 段稳定哈希
	timeline := pm.react.config.GetTimeline()
	data.TimelineFrozen = buildTimelineFrozenForPrompt(timeline)
	data.TimelineOpen = buildTimelineOpenWithMidtermForPrompt(pm.react, timeline)
	data.Timeline = pm.timelineDumpForPrompt()

	promptResult, err := pm.executeTemplate("verification", verificationPromptTemplate, data)
	return promptResult, nonce, err
}

// GenerateAIReviewPrompt generates AI tool call review prompt using template.
//
// CurrentTime 用分钟粒度: 让 BACKGROUND 段在分钟内多次调用时字节稳定,
// 配合 PROMPT_SECTION_semi-dynamic 包装使 prefix cache 能命中。
// 关键词: aicache 分钟粒度时间戳, semi-dynamic 稳定哈希
func (pm *PromptManager) GenerateAIReviewPrompt(userQuery, toolOrTitle, params string) (string, error) {
	data := &AIReviewPromptData{
		CurrentTime: time.Now().Format("2006-01-02 15:04"),
		OSArch:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:   userQuery,
		Title:       toolOrTitle,
		Details:     params,
		Nonce:       utils.RandStringBytes(4),
		Language:    pm.react.config.GetLanguage(),
	}

	// Set working directory
	data.WorkingDir = pm.workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = pm.GetGlanceWorkdir(data.WorkingDir)
	}

	// Set timeline memory
	data.Timeline = pm.timelineDumpForPrompt()

	return pm.executeTemplate("ai-review", aiReviewPromptTemplate, data)
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

// IntervalReviewPromptData contains data for interval review prompt template
type IntervalReviewPromptData struct {
	// Tool information
	ToolName        string
	ToolDescription string
	ToolParams      string

	// Timing information
	CurrentTime     string
	StartTime       string
	ElapsedDuration string
	ReviewCount     int

	// Output snapshots
	StdoutSnapshot string
	StderrSnapshot string

	// User context
	UserQuery   string
	TaskGoal    string
	TaskContext string

	// Schema
	Schema string

	CallExpectations string
	ExtraPrompt      string
	Nonce            string
}

// GenerateIntervalReviewPrompt generates interval review prompt for long-running tool execution
func (pm *PromptManager) GenerateIntervalReviewPrompt(
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
) (string, error) {
	return pm.GenerateIntervalReviewPromptWithContext(tool, params, stdoutSnapshot, stderrSnapshot, time.Time{}, 0, "")
}

// GenerateIntervalReviewPromptWithContext generates interval review prompt with additional context
func (pm *PromptManager) GenerateIntervalReviewPromptWithContext(
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	startTime time.Time,
	reviewCount int,
	callExpectations string,
) (string, error) {
	data := &IntervalReviewPromptData{
		ToolName:         tool.Name,
		ToolDescription:  tool.Description,
		ToolParams:       params.Dump(),
		CurrentTime:      time.Now().Format("2006-01-02 15:04:05"),
		StdoutSnapshot:   utils.ShrinkString(string(stdoutSnapshot), 3000),
		StderrSnapshot:   utils.ShrinkString(string(stderrSnapshot), 1500),
		Schema:           intervalReviewSchemaJSON,
		ReviewCount:      reviewCount,
		CallExpectations: callExpectations,
		ExtraPrompt:      strings.TrimSpace(pm.react.config.GetConfigString(aicommon.ConfigKeyToolCallIntervalReviewExtraPrompt)),
		Nonce:            nonce(),
	}

	// Calculate elapsed duration
	if !startTime.IsZero() {
		elapsed := time.Since(startTime)
		data.StartTime = startTime.Format("2006-01-02 15:04:05")
		data.ElapsedDuration = formatDuration(elapsed)
	} else {
		data.ElapsedDuration = "unknown"
		data.StartTime = "unknown"
	}

	// Get user query from current task
	if task := pm.react.GetCurrentTask(); task != nil {
		data.UserQuery = task.GetUserInput()
		// TaskGoal can be derived from task name or description if available
		data.TaskGoal = task.GetName()
	}

	// Get task context from timeline (truncated for prompt)
	if pm.react.config.GetTimeline() != nil {
		fullDump := pm.timelineDumpForPrompt()
		data.TaskContext = utils.ShrinkString(fullDump, 2000) // Limit to 2000 chars
	}

	return pm.executeTemplate("interval-review", intervalReviewPromptTemplate, data)
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
