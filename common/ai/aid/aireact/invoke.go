package aireact

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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
func (r *ReAct) generateMainLoopPrompt(userQuery string, tools []*aitool.Tool) string {
	var prompt bytes.Buffer

	// Background information
	prompt.WriteString("# Background\n")
	prompt.WriteString(fmt.Sprintf("Current Time: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	prompt.WriteString(fmt.Sprintf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	if cwd, err := os.Getwd(); err == nil {
		prompt.WriteString(fmt.Sprintf("Getwd: %s\n", cwd))
	}
	prompt.WriteString("\n")

	// Available tools with top N display
	if len(tools) > 0 {
		prompt.WriteString("# Available Tools\n")
		prompt.WriteString(fmt.Sprintf("You have access to %d built-in tools. Here are the top %d most important tools:\n\n", len(tools), r.config.topToolsCount))

		// Get prioritized tool list
		topTools := r.getPrioritizedTools(tools, r.config.topToolsCount)

		for _, tool := range topTools {
			prompt.WriteString(fmt.Sprintf("* `%s`: %s\n", tool.Name, tool.Description))
		}

		if len(tools) > len(topTools) {
			prompt.WriteString("...\n")
		}

		prompt.WriteString("\nUse 'tools_search' to discover additional tools for specific tasks.\n\n")
	}

	// Cumulative summary (conversation memory)
	if r.config.cumulativeSummary != "" {
		prompt.WriteString("# Conversation Memory\n")
		prompt.WriteString(r.config.cumulativeSummary + "\n\n")
	}

	// Timeline memory (replaces conversation history)
	timeline := r.config.memory.Timeline()
	if timeline != "" {
		prompt.WriteString("# Timeline Memory\n")
		prompt.WriteString(timeline)
		prompt.WriteString("\n")
	}

	// User query with nonce to prevent injection
	nonce := utils.RandStringBytes(8)
	prompt.WriteString("# User Query\n")
	prompt.WriteString(fmt.Sprintf("<|USER_QUERY_NONCE_%s|>\n", nonce))
	prompt.WriteString(userQuery + "\n")
	prompt.WriteString(fmt.Sprintf("<|USER_QUERY_NONCE_%s|>\n", nonce))
	prompt.WriteString("\n")

	// Instructions with language preference
	prompt.WriteString("# Instructions\n")

	// Language instruction
	if r.config.language == "zh" {
		prompt.WriteString("LANGUAGE: Please respond in Chinese (中文) unless specifically asked otherwise.\n")
	} else {
		prompt.WriteString("LANGUAGE: Please respond in English unless specifically asked otherwise.\n")
	}

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

	actionType := action.GetInvokeParams("next_action").GetString("type")
	if actionType == "" {
		log.Errorf("response: %s, cannot parse $..next_action.type", response)
		return nil, utils.Error("action.type is required but empty")
	}

	if !utils.StringSliceContain([]string{
		string(ActionDirectlyAnswer),
		string(ActionRequireTool),
		string(ActionRequestPlanExecution),
	}, actionType) {
		log.Errorf("response: %s, cannot parse $..next_action.type", response)
		return nil, utils.Errorf("invalid action type '%s', must be one of: %v", actionType, []any{
			ActionDirectlyAnswer,
			ActionRequireTool,
			ActionRequestPlanExecution,
		})
	}
	return action, nil
}

// executeMainLoop executes the main ReAct loop
func (r *ReAct) executeMainLoop(userQuery string, outputChan chan *ypb.AIOutputEvent) error {
	if r.config.debugEvent {
		log.Infof("executeMainLoop started with query: %s", userQuery)
		log.Infof("ReAct AI Error Learning: 已启用增强的AI错误学习功能，AI将自动从失败中学习并改进响应质量")
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

	// Initialize memory if needed
	if r.config.memory == nil {
		r.config.memory = aid.GetDefaultMemory()
	}

	// Store the user query in memory
	r.config.memory.StoreQuery(userQuery)

	// Reset iteration state for new conversation
	r.config.currentIteration = 0
	r.config.finished = false
	if r.config.debugEvent {
		log.Infof("Initialized memory with user query: %s", userQuery)
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
		prompt := r.generateMainLoopPrompt(userQuery, tools)

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
			r.emitError(outputChan, fmt.Sprintf("AI transaction failed (内置错误学习功能): %v", transactionErr))
			log.Errorf("AI transaction failed with error learning: %v", transactionErr)
			continue
		}

		if actionErr != nil {
			r.emitError(outputChan, fmt.Sprintf("Failed to parse action: %v", actionErr))
			log.Errorf("Failed to parse action: %v", actionErr)
			continue
		}

		if r.config.debugEvent {
			actionType := action.GetInvokeParams("next_action").GetString("type")
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
		actionType := ActionType(action.GetInvokeParams("next_action").GetString("type"))
		switch ActionType(actionType) {
		case ActionDirectlyAnswer:
			r.emitInfo(outputChan, "Providing direct answer")
			answerPayload := action.GetInvokeParams("next_action").GetString("answer_payload")
			r.emitAction(outputChan, fmt.Sprintf("Answer: %s", answerPayload))
			r.emitResult(outputChan, answerPayload)
			// Always mark as finished for direct answers to avoid loops
			r.config.finished = true

			// Store interaction in memory (no tool call result for direct answers)

		case ActionRequireTool:
			toolPayload := action.GetInvokeParams("next_action").GetString("tool_request_payload")
			r.emitInfo(outputChan, fmt.Sprintf("Requesting tool: %s", toolPayload))
			if err := r.handleRequireTool(toolPayload, outputChan); err != nil {
				r.emitError(outputChan, fmt.Sprintf("Tool execution failed: %v", err))
			} else {
				// Tool executed successfully, now verify if user needs are satisfied
				// Temporarily release the lock before calling verification to avoid deadlock
				r.config.mu.Unlock()
				satisfied, finalResult, err := r.verifyUserSatisfaction(userQuery, toolPayload, outputChan)
				r.config.mu.Lock()

				if err != nil {
					r.emitError(outputChan, fmt.Sprintf("Verification failed: %v", err))
					r.config.finished = true
				} else if satisfied {
					// User needs are satisfied, emit final result and finish
					r.emitResult(outputChan, finalResult)
					r.config.finished = true
				} else {
					// User needs not satisfied, continue loop
					r.emitInfo(outputChan, "User needs not fully satisfied, continuing analysis...")
				}
			}
			// Also check if explicitly marked as final step
			if action.GetBool("is_final_step") {
				r.config.finished = true
			}

		case ActionRequestPlanExecution:
			planPayload := action.GetInvokeParams("next_action").GetString("plan_request_payload")
			r.emitInfo(outputChan, fmt.Sprintf("Requesting plan execution: %s", planPayload))
			r.emitAction(outputChan, fmt.Sprintf("Plan request: %s", planPayload))
			// TODO: Implement plan execution logic

		default:
			r.emitError(outputChan, fmt.Sprintf("Unknown action type: %s", actionType))
		}

		// Timeline will automatically store tool results via handleRequireTool
		// No need to manually update conversation history as timeline handles this

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

// handleRequireTool is now implemented in invoke_toolcall.go

// generateToolParamsPrompt is now implemented in invoke_toolcall.go

// getPrioritizedTools returns a prioritized list of tools, with search tools first
func (r *ReAct) getPrioritizedTools(tools []*aitool.Tool, maxCount int) []*aitool.Tool {
	if len(tools) == 0 {
		return tools
	}

	// Priority tool names (tools_search should be first)
	priorityNames := []string{
		"tools_search",
		"now",
		"bash",
		"read_file_lines",
		"grep",
		"find_file",
		"send_http_request_by_url",
		"whois",
		"dig",
		"scan_tcp_port",
		"encode",
		"decode",
		"auto_decode",
		"current_time",
		"echo",
	}

	// Create map for quick lookup
	toolMap := make(map[string]*aitool.Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	var result []*aitool.Tool
	usedNames := make(map[string]bool)

	// Add priority tools first
	for _, name := range priorityNames {
		if tool, exists := toolMap[name]; exists && len(result) < maxCount {
			result = append(result, tool)
			usedNames[name] = true
		}
	}

	// Add remaining tools if we haven't reached maxCount
	for _, tool := range tools {
		if len(result) >= maxCount {
			break
		}
		if !usedNames[tool.Name] {
			result = append(result, tool)
		}
	}

	return result
}
