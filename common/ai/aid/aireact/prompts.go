package aireact

import (
	"bytes"
	_ "embed"
	"fmt"
	"runtime"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func nonce() string {
	return utils.RandAlphaNumStringBytes(5)
}

//go:embed prompts/tool-params/tool-params.txt
var toolParamsPromptTemplate string

//go:embed prompts/verification/verification.txt
var verificationPromptTemplate string

//go:embed prompts/verification/verification.json
var verificationSchemaJSON string

//go:embed prompts/review/ai-review-tool-call.txt
var aiReviewPromptTemplate string

//go:embed prompts/answer/directly.txt
var directlyAnswerPromptTemplate string

//go:embed prompts/tool/wrong-tool.txt
var wrongToolPromptTemplate string

//go:embed prompts/tool/wrong-params.txt
var wrongParamsPromptTemplate string

//go:embed prompts/tool-params/blueprint-params.txt
var blueprintParamsPromptTemplate string

//go:embed prompts/change-blueprint/change-blueprint.txt
var changeBlueprintPromptTemplate string

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

	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	AIForgeList        string
	Tools              []*aitool.Tool
	ToolsCount         int
	TopTools           []*aitool.Tool
	TopToolsCount      int
	HasMoreTools       bool
	ConversationMemory string
	Timeline           string
	UserQuery          string
	Nonce              string
	Language           string
	Schema             string
	DynamicContext     string
}

// ToolParamsPromptData contains data for tool parameter generation prompt
type ToolParamsPromptData struct {
	ToolName          string
	ToolDescription   string
	ToolSchema        string
	OriginalQuery     string
	CumulativeSummary string
	CurrentIteration  int
	MaxIterations     int
	Timeline          string
	DynamicContext    string
	Nonce             string   // Nonce for AITAG format
	ParamNames        []string // List of parameter names for AITAG hints
}

// VerificationPromptData contains data for verification prompt
type VerificationPromptData struct {
	Nonce          string
	OriginalQuery  string
	IsToolCall     bool
	Payload        string
	Timeline       string
	Language       string
	Schema         string
	DynamicContext string
	EnhanceData    []string
}

// AIReviewPromptData contains data for AI tool call review prompt
type AIReviewPromptData struct {
	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	ConversationMemory string
	Timeline           string
	Nonce              string
	UserQuery          string
	Title              string
	Details            string
	Language           string
}

// DirectlyAnswerPromptData contains data for directly answer prompt template
type DirectlyAnswerPromptData struct {
	AllowPlan          bool
	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	Tools              []*aitool.Tool
	ToolsCount         int
	TopTools           []*aitool.Tool
	TopToolsCount      int
	HasMoreTools       bool
	ConversationMemory string
	Timeline           string
	UserQuery          string
	Nonce              string
	Language           string
	Schema             string
	DynamicContext     string
}

// ToolReSelectPromptData contains data for tool reselection prompt template
type ToolReSelectPromptData struct {
	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	ConversationMemory string
	Timeline           string
	UserQuery          string
	Nonce              string
	OldTool            *aitool.Tool
	ToolList           []*aitool.Tool
	Schema             string
	DynamicContext     string
}

// ReGenerateToolParamsPromptData contains data for tool parameter regeneration prompt template
type ReGenerateToolParamsPromptData struct {
	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	ConversationMemory string
	Timeline           string
	UserQuery          string
	Nonce              string
	OldParams          string
	Schema             string
	DynamicContext     string
	ParamNames         []string // List of parameter names for AITAG hints
}

// AIBlueprintForgeParamsPromptData contains data for AI blueprint forge parameter generation prompt
type AIBlueprintForgeParamsPromptData struct {
	BlueprintName        string
	BlueprintDescription string
	BlueprintSchema      string
	OriginalQuery        string
	CumulativeSummary    string
	CurrentIteration     int
	MaxIterations        int
	Timeline             string
	DynamicContext       string
	OldParams            string
	ExtraPrompt          string
	Nonce                string
}

// ChangeAIBlueprintPromptData contains data for changing AI blueprint prompt template
type ChangeAIBlueprintPromptData struct {
	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	ConversationMemory string
	Timeline           string
	Nonce              string
	UserQuery          string
	CurrentBlueprint   *schema.AIForge
	ForgeList          string
	OldParams          string
	ExtraPrompt        string
	Language           string
	DynamicContext     string
}

// YaklangCodeActionLoopPromptData contains data for Yaklang code generation action loop prompt
type YaklangCodeActionLoopPromptData struct {
	CurrentTime               string
	OSArch                    string
	WorkingDir                string
	WorkingDirGlance          string
	ConversationMemory        string
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
	result["CurrentTime"] = time.Now().Format("2006-01-02 15:04:05")
	result["OSArch"] = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	result["WorkingDir"] = pm.workdir
	result["WorkingDirGlance"] = pm.GetGlanceWorkdir(pm.workdir)
	result["DynamicContext"] = pm.DynamicContext()
	result["Language"] = pm.react.config.GetLanguage()

	// Use getters instead of direct field access

	allowPlanAndExec := pm.react.config.GetEnablePlanAndExec() && pm.react.GetCurrentPlanExecutionTask() == nil

	result["AllowPlan"] = allowPlanAndExec
	if allowPlanAndExec {
		result["AIForgeList"] = pm.GetAvailableAIForgeBlueprints()
	}
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

	result["ConversationMemory"] = pm.react.cumulativeSummary
	// use timeline getter
	if t := pm.react.config.GetTimeline(); t != nil {
		result["Timeline"] = t.Dump()
	} else {
		result["Timeline"] = ""
	}
	return basePrompt, result, nil
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
		DynamicContext:  pm.DynamicContext(),
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
		}
	}

	// Extract context data from memory without lock (assume caller already holds lock)
	if t := pm.react.config.GetTimeline(); t != nil {
		if task := pm.react.GetCurrentTask(); task != nil {
			data.OriginalQuery = task.GetUserInput()
		}
		data.Timeline = t.Dump()
	}
	data.CumulativeSummary = pm.react.cumulativeSummary
	data.CurrentIteration = pm.react.currentIteration
	data.MaxIterations = int(pm.react.config.GetMaxIterations())

	prompt, err := pm.executeTemplate("tool-params", toolParamsPromptTemplate, data)
	if err != nil {
		return nil, err
	}

	return &ToolParamsPromptResult{
		Prompt:     prompt,
		Nonce:      generatedNonce,
		ParamNames: data.ParamNames,
	}, nil
}

// GenerateVerificationPrompt generates verification prompt using template
func (pm *PromptManager) GenerateVerificationPrompt(originalQuery string, isToolResult bool, payload string, enhanceData ...string) (string, string, error) {
	nonce := nonce()
	data := &VerificationPromptData{
		Nonce:          nonce,
		OriginalQuery:  originalQuery,
		IsToolCall:     isToolResult,
		Payload:        payload,
		Timeline:       "",
		Language:       pm.react.config.GetLanguage(),
		Schema:         verificationSchemaJSON,
		DynamicContext: pm.DynamicContext(),
		EnhanceData:    enhanceData,
	}

	// Get timeline for context (without lock, assume caller handles it)
	if t := pm.react.config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
	}

	promptResult, err := pm.executeTemplate("verification", verificationPromptTemplate, data)
	return promptResult, nonce, err
}

// GenerateAIReviewPrompt generates AI tool call review prompt using template
func (pm *PromptManager) GenerateAIReviewPrompt(userQuery, toolOrTitle, params string) (string, error) {
	data := &AIReviewPromptData{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
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

	// Set conversation memory
	if pm.react.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.cumulativeSummary
	}

	// Set timeline memory
	if t := pm.react.config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
	}

	return pm.executeTemplate("ai-review", aiReviewPromptTemplate, data)
}

// GenerateDirectlyAnswerPrompt generates directly answer prompt using template
func (pm *PromptManager) GenerateDirectlyAnswerPrompt(userQuery string, tools []*aitool.Tool) (string, string, error) {
	var directlyAnswerSchema = getDirectlyAnswer()

	nonceString := utils.RandStringBytes(4)
	// Build template data
	data := &DirectlyAnswerPromptData{
		AllowPlan:      false,
		CurrentTime:    time.Now().Format("2006-01-02 15:04:05"),
		OSArch:         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:      userQuery,
		Nonce:          nonceString,
		Language:       pm.react.config.GetLanguage(),
		Schema:         directlyAnswerSchema,
		Tools:          tools,
		ToolsCount:     len(tools),
		TopToolsCount:  pm.react.config.GetTopToolsCount(),
		DynamicContext: pm.DynamicContext(),
	}

	// Set working directory
	data.WorkingDir = pm.workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = pm.GetGlanceWorkdir(data.WorkingDir)
	}

	// Get prioritized tools
	if len(tools) > 0 {
		data.TopTools = pm.react.getPrioritizedTools(tools, pm.react.config.GetTopToolsCount())
		data.HasMoreTools = len(tools) > len(data.TopTools)
	}

	// Set conversation memory
	if pm.react.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.cumulativeSummary
	}

	// Set timeline memory
	if t := pm.react.config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
	}

	result, err := pm.executeTemplate("directly-answer", directlyAnswerPromptTemplate, data)
	return result, nonceString, err
}

// GenerateToolReSelectPrompt generates tool reselection prompt using template
func (pm *PromptManager) GenerateToolReSelectPrompt(noUserInteract bool, oldTool *aitool.Tool, toolList []*aitool.Tool) (string, error) {
	data := &ToolReSelectPromptData{
		CurrentTime:    time.Now().Format("2006-01-02 15:04:05"),
		OSArch:         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:      "",
		Nonce:          utils.RandStringBytes(4),
		OldTool:        oldTool,
		ToolList:       toolList,
		Schema:         getReSelectTool(noUserInteract),
		DynamicContext: pm.DynamicContext(),
	}

	if r := pm.react.GetCurrentTask(); r != nil {
		data.UserQuery = r.GetUserInput()
	}

	// Set working directory
	data.WorkingDir = pm.workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = pm.GetGlanceWorkdir(data.WorkingDir)
	}

	// Set conversation memory
	if pm.react.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.cumulativeSummary
	}

	// Set timeline memory
	if t := pm.react.config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
	}

	return pm.executeTemplate("wrong-tool", wrongToolPromptTemplate, data)
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
	data := &ReGenerateToolParamsPromptData{
		CurrentTime:    time.Now().Format("2006-01-02 15:04:05"),
		OSArch:         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:      userQuery,
		Nonce:          generatedNonce,
		OldParams:      oldParams.Dump(),
		Schema:         oldTool.ToJSONSchemaString(),
		DynamicContext: pm.DynamicContext(),
	}

	// Extract parameter names for AITAG hints
	if oldTool.Tool != nil && oldTool.Tool.InputSchema.Properties != nil {
		oldTool.Tool.InputSchema.Properties.ForEach(func(name string, _ any) bool {
			data.ParamNames = append(data.ParamNames, name)
			return true
		})
	}

	// Set working directory
	data.WorkingDir = pm.workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = pm.GetGlanceWorkdir(data.WorkingDir)
	}

	// Set conversation memory
	if pm.react.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.cumulativeSummary
	}

	// Set timeline memory
	if t := pm.react.config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
	}

	prompt, err := pm.executeTemplate("wrong-params", wrongParamsPromptTemplate, data)
	if err != nil {
		return nil, err
	}

	return &ToolParamsPromptResult{
		Prompt:     prompt,
		Nonce:      generatedNonce,
		ParamNames: data.ParamNames,
	}, nil
}

func (pm *PromptManager) GenerateChangeAIBlueprintPrompt(
	ins *schema.AIForge,
	forgeList string,
	oldParams aitool.InvokeParams,
	extraPrompt string,
) (string, error) {
	data := &ChangeAIBlueprintPromptData{
		CurrentTime:      time.Now().Format("2006-01-02 15:04:05"),
		OSArch:           fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		CurrentBlueprint: ins,
		ForgeList:        forgeList,
		ExtraPrompt:      extraPrompt,
		Nonce:            utils.RandStringBytes(4),
		Language:         pm.react.config.GetLanguage(),
		DynamicContext:   pm.DynamicContext(),
	}

	if utils.IsNil(oldParams) || len(oldParams) <= 0 {
		data.OldParams = ""
	} else {
		data.OldParams = oldParams.Dump()
	}

	// Set working directory
	data.WorkingDir = pm.workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = pm.GetGlanceWorkdir(data.WorkingDir)
	}

	// Set conversation memory
	if pm.react.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.cumulativeSummary
	}

	// Set timeline memory
	if t := pm.react.config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
		if task := pm.react.GetCurrentTask(); task != nil {
			data.UserQuery = task.GetUserInput()
		}
	}

	return pm.executeTemplate("change-blueprint", changeBlueprintPromptTemplate, data)
}

func (pm *PromptManager) GenerateAIBlueprintForgeParamsPromptEx(
	ins *schema.AIForge,
	blueprintSchema string,
	oldParams aitool.InvokeParams,
	extraPrompt string,
) (string, error) {

	data := &AIBlueprintForgeParamsPromptData{
		BlueprintName:        ins.ForgeName,
		BlueprintDescription: ins.Description,
		BlueprintSchema:      blueprintSchema,
		DynamicContext:       pm.DynamicContext(),
		ExtraPrompt:          extraPrompt,
		Nonce:                utils.RandStringBytes(4),
	}
	if utils.IsNil(oldParams) || len(oldParams) <= 0 {
		data.OldParams = ""
	} else {
		data.OldParams = oldParams.Dump()
	}

	// Extract context data from memory without lock (assume caller already holds lock)
	if t := pm.react.config.GetTimeline(); t != nil {
		if task := pm.react.GetCurrentTask(); task != nil {
			data.OriginalQuery = task.GetUserInput()
		}
		data.Timeline = t.Dump()
	}
	data.CumulativeSummary = pm.react.cumulativeSummary
	data.CurrentIteration = pm.react.currentIteration
	data.MaxIterations = int(pm.react.config.GetMaxIterations())
	return pm.executeTemplate("blueprint-params", blueprintParamsPromptTemplate, data)
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

func (pm *PromptManager) DynamicContext() string {
	return pm.cpm.Execute(pm.react.config, pm.react.config.Emitter)
}
