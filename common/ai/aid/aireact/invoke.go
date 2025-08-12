package aireact

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
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

// ReAct actions available
const (
	ReActActionObject = "object"
)

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

	// Tool capabilities overview (don't list specific tools)
	if len(tools) > 0 {
		prompt.WriteString("# Tool System\n")
		prompt.WriteString(fmt.Sprintf("You have access to %d built-in tools through the tool search system.\n", len(tools)))
		prompt.WriteString("Use 'tools_search' to discover tools for any specific task you need to accomplish.\n")
		prompt.WriteString("Tool categories include: file operations, network utilities, security testing, data processing, system commands, and more.\n\n")
	}

	// Cumulative summary (conversation memory)
	if r.config.cumulativeSummary != "" {
		prompt.WriteString("# Conversation Memory\n")
		prompt.WriteString(r.config.cumulativeSummary + "\n\n")
	}

	// Conversation history
	if len(conversationHistory) > 0 {
		prompt.WriteString("# Recent Conversation History\n")
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
	prompt.WriteString("IMPORTANT GUIDELINES:\n")
	prompt.WriteString("- Check if the user's request matches any available tool names or functionality\n")
	prompt.WriteString("- If you need to find tools for a specific task, use 'tools_search' to search available tools\n")
	prompt.WriteString("- If a tool can fulfill the request, use 'require_tool' with the exact tool name\n")
	prompt.WriteString("- For simple greetings like 'hello' or 'hi', use 'directly_answer'\n")
	prompt.WriteString("- For complex multi-step tasks, use 'request_plan_and_execution'\n")
	prompt.WriteString("- When providing direct answers, always set 'is_final_step' to true\n")
	prompt.WriteString("- TOOL SEARCH: You have access to 'tools_search' tool to find appropriate tools for any task\n")
	prompt.WriteString("- Available tool categories include: file operations, network tools, security testing, data processing, and more\n")
	prompt.WriteString("- MEMORY: Update 'cumulative_summary' to include key information from this interaction\n")
	prompt.WriteString("- The cumulative_summary should help you remember important context for future interactions\n\n")
	prompt.WriteString("Respond with a JSON object following the schema below:\n\n")

	// Schema
	prompt.WriteString("```json\n")
	prompt.WriteString(loopSchema)
	prompt.WriteString("\n```\n")

	return prompt.String()
}

// parseReActAction parses the AI response to extract the ReAct action using aid.ExtractAction
func (r *ReAct) parseReActAction(response string) (*aid.Action, error) {
	// Use aid.ExtractAction for more robust parsing
	action, err := aid.ExtractAction(response, ReActActionObject)
	if err != nil {
		return nil, utils.Errorf("failed to extract ReAct action: %v", err)
	}

	// Validate required fields
	if action.GetString("human_readable_thought") == "" {
		return nil, utils.Error("human_readable_thought is required but empty")
	}

	actionType := action.GetInvokeParams("action").GetString("type")
	if actionType == "" {
		return nil, utils.Error("action.type is required but empty")
	}

	return action, nil
}

// executeMainLoop executes the main ReAct loop
func (r *ReAct) executeMainLoop(userQuery string, outputChan chan *ypb.AIOutputEvent) error {
	if r.config.debugEvent {
		log.Infof("executeMainLoop started with query: %s", userQuery)
	}

	r.config.mu.Lock()
	defer r.config.mu.Unlock()

	// Check if finished while holding lock
	if r.config.finished {
		if r.config.debugEvent {
			log.Warn("executeMainLoop: ReAct session has finished")
		}
		return utils.Error("ReAct session has finished")
	}
	if r.config.debugEvent {
		log.Infof("executeMainLoop: session not finished, continuing")
	}

	// Initialize conversation if needed
	if len(r.config.conversationHistory) == 0 {
		r.config.conversationHistory = append(r.config.conversationHistory,
			fmt.Sprintf("User: %s", userQuery))
		r.config.currentIteration = 0
		r.config.finished = false
		if r.config.debugEvent {
			log.Infof("Initialized conversation history with: %s", userQuery)
		}
	}

	if r.config.debugEvent {
		log.Infof("executeMainLoop: starting main loop. currentIteration=%d, maxIterations=%d, finished=%t",
			r.config.currentIteration, r.config.maxIterations, r.config.finished)
	}

	for r.config.currentIteration < r.config.maxIterations && !r.config.finished {
		if r.config.debugEvent {
			log.Infof("executeMainLoop: entering loop iteration. currentIteration=%d, maxIterations=%d, finished=%t",
				r.config.currentIteration, r.config.maxIterations, r.config.finished)
		}
		r.config.currentIteration++

		if r.config.debugEvent {
			log.Infof("Starting ReAct iteration %d/%d", r.config.currentIteration, r.config.maxIterations)
		}
		r.emitIteration(outputChan, r.config.currentIteration, r.config.maxIterations)

		// Get available tools
		tools, err := r.config.aiToolManager.GetEnableTools()
		if err != nil {
			log.Errorf("Failed to get available tools: %v", err)
			return utils.Errorf("failed to get available tools: %v", err)
		}
		if r.config.debugEvent {
			log.Infof("Retrieved %d available tools", len(tools))
		}

		// Generate prompt for main loop
		conversationHistoryCopy := make([]string, len(r.config.conversationHistory))
		copy(conversationHistoryCopy, r.config.conversationHistory)
		prompt := r.generateMainLoopPrompt(userQuery, conversationHistoryCopy, tools)

		if r.config.debugPrompt {
			log.Infof("ReAct main loop prompt: %s", prompt)
		}

		// Use aid.CallAITransaction for robust AI calling with retry and error handling
		var action *aid.Action
		var actionErr error

		// Create a proper aid.Config using the public NewConfig function
		aiConfig := aid.NewConfig(r.config.ctx)

		// Temporarily release lock for AI transaction to prevent deadlocks
		r.config.mu.Unlock()

		transactionErr := aid.CallAITransaction(aiConfig, prompt,
			func(req *aid.AIRequest) (*aid.AIResponse, error) {
				// Use the stored callback
				return r.config.aiCallback(aiConfig, req)
			},
			func(resp *aid.AIResponse) error {
				// Extract response content
				responseContent := r.extractResponseContent(resp)

				if r.config.debugEvent {
					log.Infof("Attempting to parse response: %s", responseContent)
				}

				// Parse action from response using aid.ExtractAction
				action, actionErr = r.parseReActAction(responseContent)
				if actionErr != nil {
					return utils.Errorf("Failed to parse action: %v", actionErr)
				}

				return nil
			})

		// Re-acquire lock
		r.config.mu.Lock()

		if transactionErr != nil {
			r.emitError(outputChan, fmt.Sprintf("AI transaction failed: %v", transactionErr))
			log.Errorf("AI transaction failed: %v", transactionErr)
			continue
		}

		if actionErr != nil {
			r.emitError(outputChan, fmt.Sprintf("Failed to parse action: %v", actionErr))
			log.Errorf("Failed to parse action: %v", actionErr)
			continue
		}

		if r.config.debugEvent {
			actionType := action.GetInvokeParams("action").GetString("type")
			thought := action.GetString("human_readable_thought")
			log.Infof("Parsed action: type=%s, thought=%s", actionType, thought)
		}

		// Emit human readable thought
		r.emitThought(outputChan, action.GetString("human_readable_thought"))

		// Update cumulative summary for memory
		newSummary := action.GetString("cumulative_summary")
		if newSummary != "" {
			r.config.cumulativeSummary = newSummary
			if r.config.debugEvent {
				log.Infof("Updated cumulative summary: %s", newSummary)
			}
		}

		// Execute action based on type
		actionType := ActionType(action.GetInvokeParams("action").GetString("type"))
		switch actionType {
		case ActionDirectlyAnswer:
			r.emitInfo(outputChan, "Providing direct answer")
			answerPayload := action.GetInvokeParams("action").GetString("answer_payload")
			r.emitAction(outputChan, fmt.Sprintf("Answer: %s", answerPayload))
			r.emitResult(outputChan, answerPayload)
			// Always mark as finished for direct answers to avoid loops
			r.config.finished = true

			// Add to conversation history
			r.config.conversationHistory = append(r.config.conversationHistory,
				fmt.Sprintf("Assistant: %s", answerPayload))

		case ActionRequireTool:
			toolPayload := action.GetInvokeParams("action").GetString("tool_request_payload")
			r.emitInfo(outputChan, fmt.Sprintf("Requesting tool: %s", toolPayload))
			if err := r.handleRequireTool(toolPayload, outputChan); err != nil {
				r.emitError(outputChan, fmt.Sprintf("Tool execution failed: %v", err))
			} else {
				// Tool executed successfully, emit result and finish
				r.emitResult(outputChan, fmt.Sprintf("Tool %s executed successfully", toolPayload))
				r.config.finished = true
			}
			// Also check if explicitly marked as final step
			if action.GetBool("is_final_step") {
				r.config.finished = true
			}

		case ActionRequestPlanExecution:
			planPayload := action.GetInvokeParams("action").GetString("plan_request_payload")
			r.emitInfo(outputChan, fmt.Sprintf("Requesting plan execution: %s", planPayload))
			r.emitAction(outputChan, fmt.Sprintf("Plan request: %s", planPayload))
			// TODO: Implement plan execution logic

		default:
			r.emitError(outputChan, fmt.Sprintf("Unknown action type: %s", actionType))
		}

		// Update conversation history
		r.config.conversationHistory = append(r.config.conversationHistory,
			fmt.Sprintf("AI Thought: %s", action.GetString("human_readable_thought")),
			fmt.Sprintf("AI Action: %s", actionType),
		)

		// Check if final step
		if action.GetBool("is_final_step") {
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

	r.emitInfo(outputChan, fmt.Sprintf("preparing tool: %s - %s", tool.Name, tool.Description))

	// Generate tool call ID for tracking
	_ = ksuid.New().String() // callToolId for future use

	// Generate parameters for the tool
	paramsPrompt := r.generateToolParamsPrompt(tool)

	if r.config.debugPrompt {
		log.Infof("Tool params prompt: %s", paramsPrompt)
	}

	var toolParams aitool.InvokeParams

	// Use aid.CallAITransaction for tool parameter generation
	var paramsErr error
	toolConfig := aid.NewConfig(r.config.ctx)

	r.emitInfo(outputChan, "generating tool parameters...")

	err = aid.CallAITransaction(toolConfig, paramsPrompt,
		func(req *aid.AIRequest) (*aid.AIResponse, error) {
			return r.config.aiCallback(toolConfig, req)
		},
		func(resp *aid.AIResponse) error {
			// Extract parameters from response
			paramsContent := r.extractResponseContent(resp)
			toolParams, paramsErr = r.parseToolParams(paramsContent)
			if paramsErr != nil {
				return utils.Errorf("failed to parse tool parameters: %v", paramsErr)
			}
			return nil
		})

	if err != nil {
		return utils.Errorf("failed to generate tool parameters: %v", err)
	}

	if paramsErr != nil {
		return utils.Errorf("failed to parse tool parameters: %v", paramsErr)
	}

	// Format parameters for human-readable display
	var paramsList []string
	for key, value := range toolParams {
		paramsList = append(paramsList, fmt.Sprintf("%s=%v", key, value))
	}
	paramsStr := strings.Join(paramsList, ", ")

	r.emitInfo(outputChan, fmt.Sprintf("parameters generated: %s", paramsStr))

	// Execute the tool
	r.emitInfo(outputChan, fmt.Sprintf("executing tool: %s", tool.Name))

	result, err := tool.InvokeWithParams(toolParams)
	if err != nil {
		r.emitError(outputChan, fmt.Sprintf("tool execution failed: %v", err))
		return utils.Errorf("tool execution failed: %v", err)
	}

	// Emit tool result
	r.emitObservation(outputChan, fmt.Sprintf("tool %s completed, result: %s", tool.Name, result.String()))

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
