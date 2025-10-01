package aireact

import (
	"bytes"
	_ "embed"
	"fmt"
	"runtime"
	"sync"
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

// Embed template files
//
//go:embed prompts/loop/loop.txt
var loopPromptTemplate string

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

//go:embed prompts/yaklang/codeloop.txt
var yaklangCodeLoopPromptTemplate string

// PromptManager manages ReAct prompt templates
type PromptManager struct {
	cpm                  *aicommon.ContextProviderManager
	workdir              string
	glanceWorkdir        string
	genWorkdirGlanceOnce sync.Once
	react                *ReAct
}

// NewPromptManager creates a new prompt manager
func NewPromptManager(react *ReAct, workdir string) *PromptManager {
	return &PromptManager{
		cpm:                  aicommon.NewContextProviderManager(),
		workdir:              workdir,
		glanceWorkdir:        "",
		react:                react,
		genWorkdirGlanceOnce: sync.Once{},
	}
}

// LoopPromptData contains data for the main loop prompt template
type LoopPromptData struct {
	AllowAskForClarification       bool
	AllowPlan                      bool
	AllowKnowledgeEnhanceAnswer    bool
	AllowWriteYaklangCode          bool
	AskForClarificationCurrentTime int64
	AstForClarificationMaxTimes    int64

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
	EnhanceData        []string
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
	pm.genWorkdirGlanceOnce.Do(func() {
		pm.glanceWorkdir = filesys.Glance(wd)
	})
	return pm.glanceWorkdir
}

func (pm *PromptManager) GetAvailableAIForgeBlueprints() string {
	forges, err := pm.react.config.aiBlueprintManager.Query(pm.react.config.GetContext())
	if err != nil {
		log.Warnf("cannot query any ai-forge manager: %v", err)
		return ""
	}
	result, err := pm.react.config.aiBlueprintManager.GenerateAIForgeListForPrompt(forges)
	if err != nil {
		log.Warnf("cannot generate ai-forge list for prompt: %v", err)
		return ""
	}
	return result
}

// GenerateLoopPrompt generates the main ReAct loop prompt using template
func (pm *PromptManager) GenerateLoopPrompt(
	userQuery string,
	allowUserInteractive, allowPlan, allowKnowledgeEnhanceAnswer, allowWriteYaklangCode bool,
	currentUserInteractiveCount,
	userInteractiveLimitedTimes int64,
	tools []*aitool.Tool,
) (string, error) {
	forges := pm.GetAvailableAIForgeBlueprints()

	// Build template data
	data := &LoopPromptData{
		AllowAskForClarification:       allowUserInteractive,
		AllowPlan:                      allowPlan,
		AllowKnowledgeEnhanceAnswer:    allowKnowledgeEnhanceAnswer,
		AllowWriteYaklangCode:          allowWriteYaklangCode,
		AskForClarificationCurrentTime: currentUserInteractiveCount,
		AstForClarificationMaxTimes:    userInteractiveLimitedTimes,
		CurrentTime:                    time.Now().Format("2006-01-02 15:04:05"),
		OSArch:                         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:                      userQuery,
		Nonce:                          utils.RandStringBytes(4),
		Language:                       pm.react.config.language,
		AIForgeList:                    forges,
		Tools:                          tools,
		ToolsCount:                     len(tools),
		TopToolsCount:                  pm.react.config.topToolsCount,
		DynamicContext:                 pm.DynamicContext(),
	}

	data.Schema = getLoopSchema(!allowUserInteractive, !allowPlan, !allowKnowledgeEnhanceAnswer, !allowWriteYaklangCode, data.AIForgeList != "")

	data.WorkingDir = pm.workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = pm.GetGlanceWorkdir(data.WorkingDir)
	}

	// Get prioritized tools
	if len(tools) > 0 {
		data.TopTools = pm.react.getPrioritizedTools(tools, pm.react.config.topToolsCount)
		data.HasMoreTools = len(tools) > len(data.TopTools)
	}

	// Set conversation memory
	if pm.react.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.cumulativeSummary
	}

	// Set timeline memory
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("loop", loopPromptTemplate, data)
}

// GenerateToolParamsPrompt generates tool parameter generation prompt using template
func (pm *PromptManager) GenerateToolParamsPrompt(tool *aitool.Tool) (string, error) {
	data := &ToolParamsPromptData{
		ToolName:        tool.Name,
		ToolDescription: tool.Description,
		DynamicContext:  pm.DynamicContext(),
	}

	// Set tool schema if available
	if tool.Tool != nil {
		data.ToolSchema = tool.ToJSONSchemaString()
	}

	// Extract context data from memory without lock (assume caller already holds lock)
	if pm.react.config.memory != nil {
		data.OriginalQuery = pm.react.config.memory.Query
		data.Timeline = pm.react.config.memory.Timeline()
	}
	data.CumulativeSummary = pm.react.cumulativeSummary
	data.CurrentIteration = pm.react.currentIteration
	data.MaxIterations = pm.react.config.maxIterations

	return pm.executeTemplate("tool-params", toolParamsPromptTemplate, data)
}

// GenerateVerificationPrompt generates verification prompt using template
func (pm *PromptManager) GenerateVerificationPrompt(originalQuery string, isToolResult bool, payload string, enhanceData ...string) (string, error) {
	data := &VerificationPromptData{
		Nonce:          nonce(),
		OriginalQuery:  originalQuery,
		IsToolCall:     isToolResult,
		Payload:        payload,
		Timeline:       pm.react.config.memory.Timeline(),
		Language:       pm.react.config.language,
		Schema:         verificationSchemaJSON,
		DynamicContext: pm.DynamicContext(),
		EnhanceData:    enhanceData,
	}

	// Get timeline for context (without lock, assume caller handles it)
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("verification", verificationPromptTemplate, data)
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
		Language:    pm.react.config.language,
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
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("ai-review", aiReviewPromptTemplate, data)
}

// GenerateDirectlyAnswerPrompt generates directly answer prompt using template
func (pm *PromptManager) GenerateDirectlyAnswerPrompt(userQuery string, tools []*aitool.Tool, enhanceData ...string) (string, error) {
	var directlyAnswerSchema = getDirectlyAnswer()

	// Build template data
	data := &DirectlyAnswerPromptData{
		AllowPlan:      false,
		CurrentTime:    time.Now().Format("2006-01-02 15:04:05"),
		OSArch:         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:      userQuery,
		Nonce:          utils.RandStringBytes(4),
		Language:       pm.react.config.language,
		Schema:         directlyAnswerSchema,
		Tools:          tools,
		ToolsCount:     len(tools),
		TopToolsCount:  pm.react.config.topToolsCount,
		DynamicContext: pm.DynamicContext(),
		EnhanceData:    enhanceData,
	}

	// Set working directory
	data.WorkingDir = pm.workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = pm.GetGlanceWorkdir(data.WorkingDir)
	}

	// Get prioritized tools
	if len(tools) > 0 {
		data.TopTools = pm.react.getPrioritizedTools(tools, pm.react.config.topToolsCount)
		data.HasMoreTools = len(tools) > len(data.TopTools)
	}

	// Set conversation memory
	if pm.react.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.cumulativeSummary
	}

	// Set timeline memory
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("directly-answer", directlyAnswerPromptTemplate, data)
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
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("wrong-tool", wrongToolPromptTemplate, data)
}

// GenerateReGenerateToolParamsPrompt generates tool parameter regeneration prompt using template
func (pm *PromptManager) GenerateReGenerateToolParamsPrompt(userQuery string, oldParams aitool.InvokeParams, oldTool *aitool.Tool) (string, error) {
	data := &ReGenerateToolParamsPromptData{
		CurrentTime:    time.Now().Format("2006-01-02 15:04:05"),
		OSArch:         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:      userQuery,
		Nonce:          utils.RandStringBytes(4),
		OldParams:      oldParams.Dump(),
		Schema:         oldTool.ToJSONSchemaString(),
		DynamicContext: pm.DynamicContext(),
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
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("wrong-params", wrongParamsPromptTemplate, data)
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
		Language:         pm.react.config.language,
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
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
		data.UserQuery = pm.react.config.memory.Query
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
	if pm.react.config.memory != nil {
		data.OriginalQuery = pm.react.config.memory.Query
		data.Timeline = pm.react.config.memory.Timeline()
	}
	data.CumulativeSummary = pm.react.cumulativeSummary
	data.CurrentIteration = pm.react.currentIteration
	data.MaxIterations = pm.react.config.maxIterations
	return pm.executeTemplate("blueprint-params", blueprintParamsPromptTemplate, data)
}

// GenerateAIBlueprintForgeParamsPrompt generates AI blueprint forge parameter generation prompt using template
func (pm *PromptManager) GenerateAIBlueprintForgeParamsPrompt(ins *schema.AIForge, blueprintSchema string) (string, error) {
	return pm.GenerateAIBlueprintForgeParamsPromptEx(ins, blueprintSchema, nil, "")
}

// GenerateYaklangCodeActionLoop generates Yaklang code generation action loop prompt using template
func (pm *PromptManager) GenerateYaklangCodeActionLoop(
	userQuery, currentCode, errorMessages string,
	iterationCount int, tools []*aitool.Tool, nonceString string,
	allowAskForClarification bool,
	allowFinish bool,
) (string, error) {
	data := &YaklangCodeActionLoopPromptData{
		CurrentTime:               time.Now().Format("2006-01-02 15:04:05"),
		OSArch:                    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:                 userQuery,
		CurrentCode:               currentCode,
		CurrentCodeWithLineNumber: utils.PrefixLinesWithLineNumbers(currentCode),
		IterationCount:            iterationCount,
		ErrorMessages:             errorMessages,
		Nonce:                     nonceString,
		Language:                  pm.react.config.language,
		DynamicContext:            pm.DynamicContext(),
	}

	if data.Nonce == "" {
		data.Nonce = nonce()
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
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	// Get prioritized tools
	data.Tools = tools
	data.ToolsCount = len(tools)
	data.TopToolsCount = pm.react.config.topToolsCount
	if len(tools) > 0 {
		data.TopTools = pm.react.getPrioritizedTools(tools, pm.react.config.topToolsCount)
		data.HasMoreTools = len(tools) > len(data.TopTools)
	}

	// Set schema - only allow 'finish' action when there are no blocking errors
	data.Schema = getYaklangCodeLoopSchema(allowAskForClarification, allowFinish, data.Nonce)

	return pm.executeTemplate("yaklang-codeloop", yaklangCodeLoopPromptTemplate, data)
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
