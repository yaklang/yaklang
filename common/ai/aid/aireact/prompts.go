package aireact

import (
	"bytes"
	_ "embed"
	"fmt"
	"runtime"
	"sync"
	"text/template"
	"time"

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

//go:embed prompts/review/ai-review-tool-call.json
var aiReviewSchemaJSON string

//go:embed prompts/answer/directly.txt
var directlyAnswerPromptTemplate string

// PromptManager manages ReAct prompt templates
type PromptManager struct {
	workdir              string
	glanceWorkdir        string
	genWorkdirGlanceOnce sync.Once
	react                *ReAct
}

// NewPromptManager creates a new prompt manager
func NewPromptManager(react *ReAct, workdir string) *PromptManager {
	return &PromptManager{
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
	AskForClarificationCurrentTime int64
	AstForClarificationMaxTimes    int64

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
}

// VerificationPromptData contains data for verification prompt
type VerificationPromptData struct {
	Nonce         string
	OriginalQuery string
	IsToolCall    bool
	Payload       string
	Timeline      string
	Language      string
	Schema        string
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
	ToolToCall         string
	ToolParams         string
	Language           string
	Schema             string
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
}

func (pm *PromptManager) GetGlanceWorkdir(wd string) string {
	pm.genWorkdirGlanceOnce.Do(func() {
		pm.glanceWorkdir = filesys.Glance(wd)
	})
	return pm.glanceWorkdir
}

// GenerateLoopPrompt generates the main ReAct loop prompt using template
func (pm *PromptManager) GenerateLoopPrompt(
	userQuery string,
	allowUserInteractive, allowPlan bool,
	currentUserInteractiveCount,
	userInteractiveLimitedTimes int64,
	tools []*aitool.Tool,
) (string, error) {
	var loopSchema = getLoopSchema(!allowUserInteractive, !allowPlan)
	// Build template data
	data := &LoopPromptData{
		AllowAskForClarification:       allowUserInteractive,
		AllowPlan:                      allowPlan,
		AskForClarificationCurrentTime: currentUserInteractiveCount,
		AstForClarificationMaxTimes:    userInteractiveLimitedTimes,
		CurrentTime:                    time.Now().Format("2006-01-02 15:04:05"),
		OSArch:                         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:                      userQuery,
		Nonce:                          utils.RandStringBytes(4),
		Language:                       pm.react.config.language,
		Schema:                         loopSchema,
		Tools:                          tools,
		ToolsCount:                     len(tools),
		TopToolsCount:                  pm.react.config.topToolsCount,
	}

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
func (pm *PromptManager) GenerateVerificationPrompt(originalQuery string, isToolResult bool, payload string) (string, error) {
	data := &VerificationPromptData{
		Nonce:         nonce(),
		OriginalQuery: originalQuery,
		IsToolCall:    isToolResult,
		Payload:       payload,
		Timeline:      pm.react.config.memory.Timeline(),
		Language:      pm.react.config.language,
		Schema:        verificationSchemaJSON,
	}

	// Get timeline for context (without lock, assume caller handles it)
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("verification", verificationPromptTemplate, data)
}

// GenerateAIReviewPrompt generates AI tool call review prompt using template
func (pm *PromptManager) GenerateAIReviewPrompt(userQuery, toolName, toolParams string) (string, error) {
	data := &AIReviewPromptData{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
		OSArch:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:   userQuery,
		ToolToCall:  toolName,
		ToolParams:  toolParams,
		Nonce:       utils.RandStringBytes(4),
		Language:    pm.react.config.language,
		Schema:      aiReviewSchemaJSON,
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
func (pm *PromptManager) GenerateDirectlyAnswerPrompt(userQuery string, tools []*aitool.Tool) (string, error) {
	var directlyAnswerSchema = getDirectlyAnswer()

	// Build template data
	data := &DirectlyAnswerPromptData{
		AllowPlan:     false,
		CurrentTime:   time.Now().Format("2006-01-02 15:04:05"),
		OSArch:        fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:     userQuery,
		Nonce:         utils.RandStringBytes(4),
		Language:      pm.react.config.language,
		Schema:        directlyAnswerSchema,
		Tools:         tools,
		ToolsCount:    len(tools),
		TopToolsCount: pm.react.config.topToolsCount,
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
