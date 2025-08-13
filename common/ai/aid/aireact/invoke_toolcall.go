package aireact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// handleRequireTool handles tool requirement action, inspired by task_call_tool.go
func (r *ReAct) handleRequireTool(toolName string, outputChan chan *ypb.AIOutputEvent) error {
	// Find the required tool
	tool, err := r.config.aiToolManager.GetToolByName(toolName)
	if err != nil {
		return utils.Errorf("tool '%s' not found: %v", toolName, err)
	}

	r.emitInfo(outputChan, fmt.Sprintf("preparing tool: %s - %s", tool.Name, tool.Description))

	// Generate tool call ID for tracking
	callToolId := ksuid.New().String()

	// Generate parameters for the tool with improved validation
	toolParams, err := r.generateToolParams(tool, outputChan)
	if err != nil {
		return utils.Errorf("failed to generate tool parameters: %v", err)
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

	result, err := r.executeToolWithParams(tool, toolParams, callToolId)
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

// generateToolParams generates parameters for tool execution with improved validation
func (r *ReAct) generateToolParams(tool *aitool.Tool, outputChan chan *ypb.AIOutputEvent) (aitool.InvokeParams, error) {
	// Generate parameters prompt
	paramsPrompt := r.generateToolParamsPrompt(tool)

	if r.config.debugPrompt {
		log.Infof("Tool params prompt: %s", paramsPrompt)
	}

	var toolParams aitool.InvokeParams
	var paramsErr error

	// Use aid.CallAITransaction for tool parameter generation
	toolConfig := aid.NewConfig(r.config.ctx)

	r.emitInfo(outputChan, "generating tool parameters...")

	err := aid.CallAITransaction(toolConfig, paramsPrompt,
		func(req *aid.AIRequest) (*aid.AIResponse, error) {
			return r.config.aiCallback(toolConfig, req)
		},
		func(resp *aid.AIResponse) error {
			// Extract parameters from response
			paramsContent := r.extractResponseContent(resp)

			// Use improved parameter parsing with @action validation
			toolParams, paramsErr = r.parseToolParamsWithValidation(paramsContent, tool)
			if paramsErr != nil {
				return utils.Errorf("failed to parse tool parameters: %v", paramsErr)
			}
			return nil
		})

	if err != nil {
		return nil, utils.Errorf("failed to generate tool parameters: %v", err)
	}

	if paramsErr != nil {
		return nil, utils.Errorf("failed to parse tool parameters: %v", paramsErr)
	}

	return toolParams, nil
}

// parseToolParamsWithValidation parses tool parameters with enhanced validation based on aid patterns
func (r *ReAct) parseToolParamsWithValidation(response string, tool *aitool.Tool) (aitool.InvokeParams, error) {
	// Try to extract @action first for validation
	action, err := aid.ExtractAction(response, "call-tool")
	if err == nil {
		// If we found a proper @action structure, extract params from it
		params := action.GetInvokeParams("params")
		if len(params) > 0 {
			// Validate parameters against tool schema if available
			if validationErr := r.validateToolParams(params, tool); validationErr != nil {
				return nil, utils.Errorf("parameter validation failed: %v", validationErr)
			}
			return params, nil
		}
	}

	// Fallback to original JSON extraction method
	for _, pairs := range jsonextractor.ExtractObjectIndexes(response) {
		start, end := pairs[0], pairs[1]
		jsonStr := response[start:end]

		var params map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
			continue
		}

		invokeParams := aitool.InvokeParams(params)

		// Validate parameters against tool schema if available
		if validationErr := r.validateToolParams(invokeParams, tool); validationErr != nil {
			log.Warnf("Parameter validation failed for extracted JSON: %v", validationErr)
			continue
		}

		return invokeParams, nil
	}

	return nil, utils.Error("no valid parameters found in response")
}

// validateToolParams validates tool parameters against the tool's schema
func (r *ReAct) validateToolParams(params aitool.InvokeParams, tool *aitool.Tool) error {
	if tool.Tool == nil || tool.Tool.InputSchema.Properties == nil {
		// If no schema is available, we can't validate
		return nil
	}

	// Basic validation - check if required parameters are present
	if tool.Tool.InputSchema.Required != nil {
		for _, requiredParam := range tool.Tool.InputSchema.Required {
			if !params.Has(requiredParam) {
				return utils.Errorf("required parameter '%s' is missing", requiredParam)
			}
		}
	}

	// Additional validation can be added here based on schema types
	// For now, we'll do basic presence validation

	return nil
}

// executeToolWithParams executes a tool with the given parameters
func (r *ReAct) executeToolWithParams(tool *aitool.Tool, params aitool.InvokeParams, callToolId string) (*aitool.ToolResult, error) {
	// Execute the tool
	result, err := tool.InvokeWithParams(params)
	if err != nil {
		return nil, utils.Errorf("tool execution failed: %v", err)
	}

	// Set additional metadata on the result
	if result != nil {
		result.ToolCallID = callToolId
	}

	return result, nil
}

// generateToolParamsPrompt generates prompt for tool parameter generation with enhanced schema information
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

	// Extract context data - NOTE: This method should be called when the caller already holds the lock
	// or when lock is not needed (data is passed as parameters)
	originalQuery := r.extractOriginalUserQueryUnsafe()
	cumulativeSummary := r.config.cumulativeSummary
	currentIteration := r.config.currentIteration
	maxIterations := r.config.maxIterations
	conversationHistory := make([]string, len(r.config.conversationHistory))
	copy(conversationHistory, r.config.conversationHistory)

	// Original user query for context
	if originalQuery != "" {
		prompt.WriteString("# Original User Query\n")
		prompt.WriteString(fmt.Sprintf("User's original request: %s\n\n", originalQuery))
	}

	// Cumulative summary for overall context and memory
	if cumulativeSummary != "" {
		prompt.WriteString("# Task Context & Memory\n")
		prompt.WriteString(fmt.Sprintf("Overall task context: %s\n\n", cumulativeSummary))
	}

	// Current task progress
	prompt.WriteString("# Task Progress\n")
	prompt.WriteString(fmt.Sprintf("Current iteration: %d/%d\n", currentIteration, maxIterations))
	prompt.WriteString("This tool call is part of a multi-step ReAct process. Consider how this tool execution contributes to completing the overall user task.\n\n")

	// Recent conversation context
	if len(conversationHistory) > 0 {
		prompt.WriteString("# Recent Conversation\n")
		recentHistory := conversationHistory
		if len(recentHistory) > 8 {
			recentHistory = recentHistory[len(recentHistory)-8:]
		}
		for _, entry := range recentHistory {
			prompt.WriteString(entry + "\n")
		}
		prompt.WriteString("\n")
	}

	// Enhanced instructions for parameter generation with task completion context
	prompt.WriteString("# Instructions\n")
	prompt.WriteString("Generate appropriate parameters for this tool call based on the context above.\n\n")
	prompt.WriteString("IMPORTANT CONSIDERATIONS:\n")
	prompt.WriteString("- Consider the original user query and overall task context when generating parameters\n")
	prompt.WriteString("- This tool execution should contribute to completing the user's original request\n")
	prompt.WriteString("- The cumulative summary contains the evolving context - use this to understand what has been accomplished so far\n")
	prompt.WriteString("- After this tool execution, the ReAct loop will determine if the task is complete or if further actions are needed\n")
	prompt.WriteString("- Generate parameters that will produce meaningful results toward task completion\n\n")
	prompt.WriteString("RESPONSE FORMAT: Respond with a JSON object following the @action pattern:\n\n")
	prompt.WriteString("```json\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"@action\": \"call-tool\",\n")
	prompt.WriteString("  \"params\": {\n")
	prompt.WriteString("    \"param1\": \"value1\",\n")
	prompt.WriteString("    \"param2\": \"value2\"\n")
	prompt.WriteString("  }\n")
	prompt.WriteString("}\n")
	prompt.WriteString("```\n\n")

	return prompt.String()
}

// extractOriginalUserQuery extracts the original user query from conversation history (thread-safe)
func (r *ReAct) extractOriginalUserQuery() string {
	r.config.mu.RLock()
	defer r.config.mu.RUnlock()
	return r.extractOriginalUserQueryUnsafe()
}

// extractOriginalUserQueryUnsafe extracts the original user query from conversation history (NOT thread-safe)
// This method should only be called when the caller already holds the lock
func (r *ReAct) extractOriginalUserQueryUnsafe() string {
	if len(r.config.conversationHistory) == 0 {
		return ""
	}

	// Look for the first "User:" entry in conversation history
	for _, entry := range r.config.conversationHistory {
		if strings.HasPrefix(entry, "User: ") {
			return strings.TrimPrefix(entry, "User: ")
		}
	}

	return ""
}

// verifyUserSatisfaction verifies if the tool execution satisfied the user's needs and provides human-readable output
func (r *ReAct) verifyUserSatisfaction(originalQuery, toolName string, outputChan chan *ypb.AIOutputEvent) (bool, string, error) {
	// Generate verification prompt
	verificationPrompt := r.generateVerificationPrompt(originalQuery, toolName)

	if r.config.debugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	var satisfied bool
	var finalResult string
	var verificationErr error

	// Use aid.CallAITransaction for verification
	toolConfig := aid.NewConfig(r.config.ctx)

	r.emitInfo(outputChan, "Verifying if user needs are satisfied and formatting results...")

	err := aid.CallAITransaction(toolConfig, verificationPrompt,
		func(req *aid.AIRequest) (*aid.AIResponse, error) {
			return r.config.aiCallback(toolConfig, req)
		},
		func(resp *aid.AIResponse) error {
			// Extract verification response
			responseContent := r.extractResponseContent(resp)

			// Parse verification result
			satisfied, finalResult, verificationErr = r.parseVerificationResponse(responseContent)
			if verificationErr != nil {
				return utils.Errorf("failed to parse verification response: %v", verificationErr)
			}
			return nil
		})

	if err != nil {
		return false, "", utils.Errorf("failed to verify user satisfaction: %v", err)
	}

	if verificationErr != nil {
		return false, "", utils.Errorf("failed to parse verification response: %v", verificationErr)
	}

	return satisfied, finalResult, nil
}

// generateVerificationPrompt generates a prompt for verifying user satisfaction
func (r *ReAct) generateVerificationPrompt(originalQuery, toolName string) string {
	var prompt bytes.Buffer

	prompt.WriteString("# Task Verification and Result Formatting\n\n")
	prompt.WriteString("You are tasked with verifying if a tool execution has satisfied the user's original request and providing a human-readable summary.\n\n")

	// Original user query
	prompt.WriteString("# Original User Query\n")
	prompt.WriteString(fmt.Sprintf("User's request: %s\n\n", originalQuery))

	// Tool execution context
	prompt.WriteString("# Tool Execution Context\n")
	prompt.WriteString(fmt.Sprintf("Tool executed: %s\n", toolName))

	// Recent conversation history for context
	r.config.mu.RLock()
	conversationHistory := make([]string, len(r.config.conversationHistory))
	copy(conversationHistory, r.config.conversationHistory)
	r.config.mu.RUnlock()

	if len(conversationHistory) > 0 {
		prompt.WriteString("# Recent Conversation History\n")
		// Show last few entries for context
		recentHistory := conversationHistory
		if len(recentHistory) > 6 {
			recentHistory = recentHistory[len(recentHistory)-6:]
		}
		for _, entry := range recentHistory {
			prompt.WriteString(entry + "\n")
		}
		prompt.WriteString("\n")
	}

	// Language preference
	if r.config.language == "zh" {
		prompt.WriteString("LANGUAGE: Please respond in Chinese (中文).\n\n")
	} else {
		prompt.WriteString("LANGUAGE: Please respond in English.\n\n")
	}

	// Instructions
	prompt.WriteString("# Instructions\n")
	prompt.WriteString("Based on the tool execution results and conversation history, determine:\n")
	prompt.WriteString("1. Whether the user's original request has been satisfied\n")
	prompt.WriteString("2. Provide a human-readable summary of the results\n\n")

	prompt.WriteString("RESPONSE FORMAT: Respond with a JSON object:\n\n")
	prompt.WriteString("```json\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"@action\": \"verify-satisfaction\",\n")
	prompt.WriteString("  \"user_satisfied\": true/false,\n")
	prompt.WriteString("  \"human_readable_result\": \"Clear, concise summary of what was accomplished and any key findings\",\n")
	prompt.WriteString("  \"reasoning\": \"Brief explanation of why the user's needs are/aren't satisfied\"\n")
	prompt.WriteString("}\n")
	prompt.WriteString("```\n\n")

	prompt.WriteString("IMPORTANT GUIDELINES:\n")
	prompt.WriteString("- Set user_satisfied to true only if the original request was genuinely fulfilled\n")
	prompt.WriteString("- If the tool failed or produced unclear results, set user_satisfied to false\n")
	prompt.WriteString("- Make the human_readable_result clear and informative for the user\n")
	prompt.WriteString("- Focus on what the user actually wanted to know or accomplish\n")

	return prompt.String()
}

// parseVerificationResponse parses the verification response to extract satisfaction status and human-readable result
func (r *ReAct) parseVerificationResponse(response string) (bool, string, error) {
	// Try to extract @action first for validation
	action, err := aid.ExtractAction(response, "verify-satisfaction")
	if err != nil {
		return false, "", utils.Errorf("failed to extract verification action: %v", err)
	}

	// Extract satisfaction status
	satisfied := action.GetBool("user_satisfied")

	// Extract human-readable result
	humanReadableResult := action.GetString("human_readable_result")
	if humanReadableResult == "" {
		return false, "", utils.Error("human_readable_result is required but empty")
	}

	// Optional: extract reasoning for debugging
	reasoning := action.GetString("reasoning")
	if r.config.debugEvent && reasoning != "" {
		log.Infof("Verification reasoning: %s", reasoning)
	}

	return satisfied, humanReadableResult, nil
}
