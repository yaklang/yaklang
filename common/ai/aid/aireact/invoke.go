package aireact

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed aireact.schema.json
var loopSchema string

// ActionType represents the type of action to take
type ActionType string

const (
	ActionDirectlyAnswer       ActionType = "directly_answer"
	ActionRequireTool          ActionType = "require_tool"
	ActionRequestPlanExecution ActionType = "request_plan_and_execution"
)

// ReActAction represents the parsed action from AI response
type ReActAction struct {
	Type                 ActionType `json:"type"`
	AnswerPayload        string     `json:"answer_payload,omitempty"`
	ToolRequestPayload   string     `json:"tool_request_payload,omitempty"`
	PlanRequestPayload   string     `json:"plan_request_payload,omitempty"`
	HumanReadableThought string     `json:"human_readable_thought"`
	CumulativeSummary    string     `json:"cumulative_summary"`
	IsFinalStep          bool       `json:"is_final_step"`
}

// generateMainLoopPrompt generates the prompt for the main ReAct loop
func (r *ReAct) generateMainLoopPrompt(userQuery string, conversationHistory []string, tools []*aitool.Tool) string {
	var prompt bytes.Buffer

	// Background information
	prompt.WriteString("# Background\n")
	prompt.WriteString(fmt.Sprintf("Current Time: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	prompt.WriteString(fmt.Sprintf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	if cwd, err := os.Getwd(); err == nil {
		prompt.WriteString(fmt.Sprintf("Getwd: %s\n", cwd))
	}
	prompt.WriteString("\n")

	// Available tools
	if len(tools) > 0 {
		prompt.WriteString("# Available Tools\n")
		for _, tool := range tools {
			prompt.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description))
		}
		prompt.WriteString("\n")
	}

	// Conversation history
	if len(conversationHistory) > 0 {
		prompt.WriteString("# Conversation History\n")
		for _, entry := range conversationHistory {
			prompt.WriteString(entry + "\n")
		}
		prompt.WriteString("\n")
	}

	// User query with nonce to prevent injection
	nonce := utils.RandStringBytes(8)
	prompt.WriteString("# User Query\n")
	prompt.WriteString(fmt.Sprintf("<|USER_QUERY_NONCE_%s|>\n", nonce))
	prompt.WriteString(userQuery + "\n")
	prompt.WriteString(fmt.Sprintf("<|USER_QUERY_NONCE_%s|>\n", nonce))
	prompt.WriteString("\n")

	// Instructions
	prompt.WriteString("# Instructions\n")
	prompt.WriteString("You are a ReAct (Reasoning and Acting) AI agent. Analyze the user query and decide what action to take.\n")
	prompt.WriteString("Respond with a JSON object following the schema below:\n\n")

	// Schema
	prompt.WriteString("```json\n")
	prompt.WriteString(loopSchema)
	prompt.WriteString("\n```\n")

	return prompt.String()
}

// parseReActAction parses the AI response to extract the ReAct action
func (r *ReAct) parseReActAction(response string) (*ReActAction, error) {
	// Extract JSON objects from response
	for _, pairs := range jsonextractor.ExtractObjectIndexes(response) {
		start, end := pairs[0], pairs[1]
		jsonStr := response[start:end]

		var action ReActAction
		if err := json.Unmarshal([]byte(jsonStr), &action); err != nil {
			continue
		}

		// Validate required fields
		if action.Type != "" && action.HumanReadableThought != "" {
			return &action, nil
		}
	}

	return nil, utils.Error("no valid ReAct action found in response")
}

// executeMainLoop executes the main ReAct loop
func (r *ReAct) executeMainLoop(userQuery string, outputChan chan *ypb.AIOutputEvent) error {
	r.config.mu.Lock()
	defer r.config.mu.Unlock()

	if r.config.finished {
		return utils.Error("ReAct session has finished")
	}

	// Initialize conversation if needed
	if len(r.config.conversationHistory) == 0 {
		r.config.conversationHistory = append(r.config.conversationHistory,
			fmt.Sprintf("User: %s", userQuery))
	}

	for r.config.currentIteration < r.config.maxIterations && !r.config.finished {
		r.config.currentIteration++

		r.emitIteration(outputChan, r.config.currentIteration, r.config.maxIterations)

		// Get available tools
		tools, err := r.config.aiToolManager.GetEnableTools()
		if err != nil {
			return utils.Errorf("failed to get available tools: %v", err)
		}

		// Generate prompt for main loop
		prompt := r.generateMainLoopPrompt(userQuery, r.config.conversationHistory, tools)

		if r.config.debugPrompt {
			log.Infof("ReAct main loop prompt: %s", prompt)
		}

		// Call AI to get next action
		req := aid.NewAIRequest(prompt)
		resp, err := r.config.aiCallback(nil, req) // Pass nil config for now
		if err != nil {
			return utils.Errorf("AI callback failed: %v", err)
		}

		// Extract response content
		responseContent := r.extractResponseContent(resp)

		// Parse action from response
		action, err := r.parseReActAction(responseContent)
		if err != nil {
			r.emitError(outputChan, fmt.Sprintf("Failed to parse action: %v", err))
			continue
		}

		// Emit human readable thought
		r.emitThought(outputChan, action.HumanReadableThought)

		// Execute action based on type
		switch action.Type {
		case ActionDirectlyAnswer:
			r.emitInfo(outputChan, "Providing direct answer")
			r.emitAction(outputChan, fmt.Sprintf("Answer: %s", action.AnswerPayload))
			r.emitResult(outputChan, action.AnswerPayload)
			r.config.finished = action.IsFinalStep

		case ActionRequireTool:
			r.emitInfo(outputChan, fmt.Sprintf("Requesting tool: %s", action.ToolRequestPayload))
			if err := r.handleRequireTool(action.ToolRequestPayload, outputChan); err != nil {
				r.emitError(outputChan, fmt.Sprintf("Tool execution failed: %v", err))
			}

		case ActionRequestPlanExecution:
			r.emitInfo(outputChan, fmt.Sprintf("Requesting plan execution: %s", action.PlanRequestPayload))
			r.emitAction(outputChan, fmt.Sprintf("Plan request: %s", action.PlanRequestPayload))
			// TODO: Implement plan execution logic

		default:
			r.emitError(outputChan, fmt.Sprintf("Unknown action type: %s", action.Type))
		}

		// Update conversation history
		r.config.conversationHistory = append(r.config.conversationHistory,
			fmt.Sprintf("AI Thought: %s", action.HumanReadableThought),
			fmt.Sprintf("AI Action: %s", string(action.Type)),
		)

		// Check if final step
		if action.IsFinalStep {
			r.config.finished = true
			r.emitInfo(outputChan, "ReAct main loop completed")
			break
		}
	}

	if r.config.currentIteration >= r.config.maxIterations {
		r.emitInfo(outputChan, "ReAct loop reached maximum iterations")
	}

	return nil
}

// handleRequireTool handles tool requirement action, inspired by task_call_tool.go
func (r *ReAct) handleRequireTool(toolName string, outputChan chan *ypb.AIOutputEvent) error {
	// Find the required tool
	tool, err := r.config.aiToolManager.GetToolByName(toolName)
	if err != nil {
		return utils.Errorf("tool '%s' not found: %v", toolName, err)
	}

	r.emitInfo(outputChan, fmt.Sprintf("Found tool: %s", tool.Name))

	// Generate tool call ID for tracking
	_ = ksuid.New().String() // callToolId for future use

	// Generate parameters for the tool
	paramsPrompt := r.generateToolParamsPrompt(tool)

	if r.config.debugPrompt {
		log.Infof("Tool params prompt: %s", paramsPrompt)
	}

	var toolParams aitool.InvokeParams

	// Call AI to generate tool parameters
	req := aid.NewAIRequest(paramsPrompt)
	resp, err := r.config.aiCallback(nil, req) // Pass nil config for now
	if err != nil {
		return utils.Errorf("failed to generate tool parameters: %v", err)
	}

	// Extract parameters from response
	paramsContent := r.extractResponseContent(resp)
	toolParams, err = r.parseToolParams(paramsContent)
	if err != nil {
		return utils.Errorf("failed to parse tool parameters: %v", err)
	}

	r.emitInfo(outputChan, fmt.Sprintf("Generated parameters for tool %s", tool.Name))

	// Execute the tool
	result, err := tool.InvokeWithParams(toolParams)
	if err != nil {
		return utils.Errorf("tool execution failed: %v", err)
	}

	// Emit tool result
	r.emitObservation(outputChan, fmt.Sprintf("Tool %s result: %s", tool.Name, result.String()))

	// Add tool call to conversation history
	r.config.conversationHistory = append(r.config.conversationHistory,
		fmt.Sprintf("Tool Call: %s with params %v", tool.Name, toolParams),
		fmt.Sprintf("Tool Result: %s", result.String()),
	)

	return nil
}

// generateToolParamsPrompt generates prompt for tool parameter generation
func (r *ReAct) generateToolParamsPrompt(tool *aitool.Tool) string {
	var prompt bytes.Buffer

	prompt.WriteString("# Tool Parameter Generation\n\n")
	prompt.WriteString(fmt.Sprintf("You need to generate parameters for the tool '%s'.\n\n", tool.Name))
	prompt.WriteString(fmt.Sprintf("Tool Description: %s\n\n", tool.Description))

	// Tool schema (if available)
	if tool.Tool != nil && tool.Tool.InputSchema.Properties != nil {
		schemaJson, _ := json.MarshalIndent(tool.Tool.InputSchema, "", "  ")
		prompt.WriteString("Tool Schema:\n")
		prompt.WriteString("```json\n")
		prompt.WriteString(string(schemaJson))
		prompt.WriteString("\n```\n\n")
	}

	// Recent conversation context
	if len(r.config.conversationHistory) > 0 {
		prompt.WriteString("Recent Conversation:\n")
		recentHistory := r.config.conversationHistory
		if len(recentHistory) > 5 {
			recentHistory = recentHistory[len(recentHistory)-5:]
		}
		for _, entry := range recentHistory {
			prompt.WriteString(entry + "\n")
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("Generate appropriate parameters for this tool call and respond with a JSON object containing the parameters.\n")
	prompt.WriteString("Example: {\"param1\": \"value1\", \"param2\": \"value2\"}\n")

	return prompt.String()
}

// parseToolParams parses tool parameters from AI response
func (r *ReAct) parseToolParams(response string) (aitool.InvokeParams, error) {
	// Extract JSON objects from response
	for _, pairs := range jsonextractor.ExtractObjectIndexes(response) {
		start, end := pairs[0], pairs[1]
		jsonStr := response[start:end]

		var params map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
			continue
		}

		return aitool.InvokeParams(params), nil
	}

	return nil, utils.Error("no valid parameters found in response")
}
