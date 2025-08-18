package aireact

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// Embed template files
//
//go:embed prompts/loop/loop.txt
var loopPromptTemplate string

//go:embed prompts/loop/loop.json
var loopSchemaJSON string

//go:embed prompts/tool-params/tool-params.txt
var toolParamsPromptTemplate string

//go:embed prompts/verification/verification.txt
var verificationPromptTemplate string

//go:embed prompts/verification/verification.json
var verificationSchemaJSON string

// PromptManager manages ReAct prompt templates
type PromptManager struct {
	react *ReAct
}

// NewPromptManager creates a new prompt manager
func NewPromptManager(react *ReAct) *PromptManager {
	return &PromptManager{react: react}
}

// LoopPromptData contains data for the main loop prompt template
type LoopPromptData struct {
	CurrentTime        string
	OSArch             string
	WorkingDir         string
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
	OriginalQuery string
	ToolName      string
	Timeline      string
	Language      string
	Schema        string
}

// GenerateLoopPrompt generates the main ReAct loop prompt using template
func (pm *PromptManager) GenerateLoopPrompt(userQuery string, tools []*aitool.Tool) (string, error) {
	// Build template data
	data := &LoopPromptData{
		CurrentTime:   time.Now().Format("2006-01-02 15:04:05"),
		OSArch:        fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:     userQuery,
		Nonce:         utils.RandStringBytes(8),
		Language:      pm.react.config.language,
		Schema:        loopSchemaJSON,
		Tools:         tools,
		ToolsCount:    len(tools),
		TopToolsCount: pm.react.config.topToolsCount,
	}

	// Set working directory
	if cwd, err := os.Getwd(); err == nil {
		data.WorkingDir = cwd
	}

	// Get prioritized tools
	if len(tools) > 0 {
		data.TopTools = pm.react.getPrioritizedTools(tools, pm.react.config.topToolsCount)
		data.HasMoreTools = len(tools) > len(data.TopTools)
	}

	// Set conversation memory
	if pm.react.config.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.config.cumulativeSummary
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
	data.CumulativeSummary = pm.react.config.cumulativeSummary
	data.CurrentIteration = pm.react.config.currentIteration
	data.MaxIterations = pm.react.config.maxIterations

	return pm.executeTemplate("tool-params", toolParamsPromptTemplate, data)
}

// GenerateVerificationPrompt generates verification prompt using template
func (pm *PromptManager) GenerateVerificationPrompt(originalQuery, toolName string) (string, error) {
	data := &VerificationPromptData{
		OriginalQuery: originalQuery,
		ToolName:      toolName,
		Language:      pm.react.config.language,
		Schema:        verificationSchemaJSON,
	}

	// Get timeline for context (without lock, assume caller handles it)
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("verification", verificationPromptTemplate, data)
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
