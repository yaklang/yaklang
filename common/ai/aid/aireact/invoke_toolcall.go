package aireact

import (
	"encoding/json"
	"fmt"
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

	// Tool use review logic (similar to task_call_tool.go)
	if tool.NoNeedUserReview {
		r.emitInfo(outputChan, fmt.Sprintf("tool[%v] (internal helper tool) no need user review, skip review", tool.Name))
	} else if r.config.enableToolReview {
		r.emitInfo(outputChan, "start to require review for tool use")

		// Handle tool use review
		finalTool, finalParams, overrideResult, shouldDirectlyAnswer, err := r.handleToolUseReview(tool, toolParams, outputChan)
		if err != nil {
			r.emitError(outputChan, fmt.Sprintf("error handling tool use review: %v", err))
			return utils.Errorf("error handling tool use review: %v", err)
		}

		// Handle review results
		if overrideResult != nil {
			// Store overridden result in timeline memory
			r.config.memory.PushToolCallResults(overrideResult)
			r.emitObservation(outputChan, fmt.Sprintf("tool %s overridden with result: %s", tool.Name, overrideResult.String()))
			return nil
		}

		if shouldDirectlyAnswer {
			r.emitObservation(outputChan, "user requested direct answer without tool execution")
			return nil
		}

		// Update tool and params based on review
		tool = finalTool
		toolParams = finalParams
	}

	// Execute the tool
	r.emitInfo(outputChan, fmt.Sprintf("executing tool: %s", tool.Name))

	result, err := r.executeToolWithParams(tool, toolParams, callToolId)
	if err != nil {
		r.emitError(outputChan, fmt.Sprintf("tool execution failed: %v", err))
		return utils.Errorf("tool execution failed: %v", err)
	}

	// Emit tool result
	r.emitObservation(outputChan, fmt.Sprintf("tool %s completed, result: %s", tool.Name, result.String()))

	// Store tool result in timeline memory
	r.config.memory.PushToolCallResults(result)

	return nil
}

// generateToolParams generates parameters for tool execution with improved validation
func (r *ReAct) generateToolParams(tool *aitool.Tool, outputChan chan *ypb.AIOutputEvent) (aitool.InvokeParams, error) {
	// Generate parameters prompt
	paramsPrompt := r.generateToolParamsPrompt(tool)

	if r.config.debugPrompt {
		log.Infof("Tool params prompt: %s", paramsPrompt)
	}

	r.emitInfo(outputChan, "generating tool parameters...")

	// Create a simple AI request and call the ReAct's aiCallback directly to use breakpoint functionality
	req := aid.NewAIRequest(paramsPrompt)
	resp, err := r.config.aiCallback(nil, req)
	if err != nil {
		return nil, utils.Errorf("failed to generate tool parameters: %v", err)
	}

	// Extract parameters from response
	paramsContent := r.extractResponseContent(resp)

	// Display AI response content for parameter generation
	r.emitInfo(outputChan, fmt.Sprintf("AI response for parameter generation: %s", paramsContent))

	// Use improved parameter parsing with @action validation
	toolParams, err := r.parseToolParamsWithValidation(paramsContent, tool)
	if err != nil {
		return nil, utils.Errorf("failed to parse tool parameters: %v", err)
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
	// Use the prompt manager to generate the prompt
	prompt, err := r.promptManager.GenerateToolParamsPrompt(tool)
	if err != nil {
		// Fallback to basic prompt if template fails
		log.Errorf("Failed to generate tool params prompt from template: %v", err)
		return fmt.Sprintf("Generate parameters for tool '%s': %s", tool.Name, tool.Description)
	}
	return prompt
}

// handleToolUseReview handles tool use review process, inspired by aid's handleToolUseReview
func (r *ReAct) handleToolUseReview(tool *aitool.Tool, params aitool.InvokeParams, outputChan chan *ypb.AIOutputEvent) (*aitool.Tool, aitool.InvokeParams, *aitool.ToolResult, bool, error) {
	// If custom review handler is provided, use it
	if r.config.reviewHandler != nil {
		return r.handleCustomToolReview(tool, params, outputChan)
	}

	// Default implementation: emit review requirement event and wait for response
	reviewID := ksuid.New().String()

	// Emit tool use review requirement event
	r.emitToolUseReviewRequire(tool, params, reviewID, outputChan)

	// For now, we'll use a simplified default behavior
	// In a full implementation, this would wait for user input
	// Here we default to "continue" for basic functionality
	r.emitInfo(outputChan, "tool use review approved (default behavior)")

	return tool, params, nil, false, nil
}

// handleCustomToolReview handles tool review using custom review handler
func (r *ReAct) handleCustomToolReview(tool *aitool.Tool, params aitool.InvokeParams, outputChan chan *ypb.AIOutputEvent) (*aitool.Tool, aitool.InvokeParams, *aitool.ToolResult, bool, error) {
	reviewInfo := &ToolReviewInfo{
		Tool:            tool,
		Params:          params,
		ID:              ksuid.New().String(),
		ResponseChannel: make(chan *ToolReviewResponse, 1),
	}

	// Call custom review handler in a goroutine
	go r.config.reviewHandler(reviewInfo)

	// Wait for review response with timeout
	select {
	case response := <-reviewInfo.ResponseChannel:
		return r.processReviewResponse(tool, params, response, outputChan)
	case <-r.config.ctx.Done():
		return tool, params, nil, false, utils.Error("review cancelled due to context cancellation")
	}
}

// processReviewResponse processes the review response and returns appropriate action
func (r *ReAct) processReviewResponse(tool *aitool.Tool, params aitool.InvokeParams, response *ToolReviewResponse, outputChan chan *ypb.AIOutputEvent) (*aitool.Tool, aitool.InvokeParams, *aitool.ToolResult, bool, error) {
	if response.Cancel {
		r.emitInfo(outputChan, "tool use cancelled by user")
		return tool, params, nil, false, utils.Error("tool use cancelled by user")
	}

	if response.DirectlyAnswer {
		r.emitInfo(outputChan, "user requested direct answer without tool execution")
		return tool, params, nil, true, nil
	}

	if response.OverrideResult != nil {
		r.emitInfo(outputChan, "tool result overridden by user")
		return tool, params, response.OverrideResult, false, nil
	}

	switch response.Suggestion {
	case "continue":
		r.emitInfo(outputChan, "tool use approved by user")
		return tool, params, nil, false, nil

	case "wrong_tool":
		r.emitInfo(outputChan, "user suggests wrong tool, attempting to reselect")
		newTool, err := r.handleWrongToolSuggestion(tool, response.SuggestionTool, response.SuggestionKeyword, outputChan)
		if err != nil {
			return tool, params, nil, false, err
		}
		return newTool, params, nil, false, nil

	case "wrong_params":
		r.emitInfo(outputChan, "user suggests wrong parameters")
		if response.ModifiedParams != nil {
			r.emitInfo(outputChan, "using user-modified parameters")
			return tool, response.ModifiedParams, nil, false, nil
		}
		return tool, params, nil, false, nil

	case "direct_answer":
		r.emitInfo(outputChan, "user requested direct answer")
		return tool, params, nil, true, nil

	default:
		r.emitError(outputChan, fmt.Sprintf("unknown review suggestion: %s", response.Suggestion))
		return tool, params, nil, false, utils.Errorf("unknown review suggestion: %s", response.Suggestion)
	}
}

// handleWrongToolSuggestion handles tool reselection when user suggests wrong tool
func (r *ReAct) handleWrongToolSuggestion(oldTool *aitool.Tool, suggestionTool, suggestionKeyword string, outputChan chan *ypb.AIOutputEvent) (*aitool.Tool, error) {
	var tools []*aitool.Tool

	// Try to find suggested tool by name
	if suggestionTool != "" {
		for _, toolName := range strings.Split(suggestionTool, ",") {
			toolName = strings.TrimSpace(toolName)
			if toolIns, err := r.config.aiToolManager.GetToolByName(toolName); err == nil && toolIns != nil {
				tools = append(tools, toolIns)
			} else {
				r.emitInfo(outputChan, fmt.Sprintf("suggested tool '%s' not found", toolName))
			}
		}
	}

	// Search by keyword if provided
	if suggestionKeyword != "" {
		searched, err := r.config.aiToolManager.SearchTools("", suggestionKeyword)
		if err != nil {
			r.emitError(outputChan, fmt.Sprintf("error searching tools: %v", err))
		} else {
			tools = append(tools, searched...)
		}
	}

	if len(tools) == 0 {
		return oldTool, utils.Error("no suitable tools found based on user suggestion")
	}

	// For simplicity, return the first found tool
	// In a full implementation, this could involve AI-based selection
	selectedTool := tools[0]
	r.emitInfo(outputChan, fmt.Sprintf("reselected tool: %s", selectedTool.Name))

	return selectedTool, nil
}

// ExampleToolReviewHandler demonstrates how to implement a custom tool review handler
func ExampleToolReviewHandler(reviewInfo *ToolReviewInfo) {
	log.Infof("Tool review request received for: %s", reviewInfo.Tool.Name)
	log.Infof("Parameters: %v", reviewInfo.Params)

	// In a real implementation, this would interact with a user interface
	// For demonstration, we'll auto-approve with logging
	response := &ToolReviewResponse{
		Suggestion: "continue", // Options: continue, wrong_tool, wrong_params, direct_answer
		// ExtraPrompt: "Additional user guidance",
		// SuggestionTool: "alternative_tool_name",
		// SuggestionKeyword: "search keyword",
		// ModifiedParams: modified parameters map,
		// OverrideResult: custom result,
		// DirectlyAnswer: true/false,
		// Cancel: true/false,
	}

	// Send response back
	select {
	case reviewInfo.ResponseChannel <- response:
		log.Infof("Tool review response sent: %s", response.Suggestion)
	default:
		log.Warnf("Failed to send tool review response - channel may be closed")
	}
}

// emitToolUseReviewRequire emits a tool use review requirement event
func (r *ReAct) emitToolUseReviewRequire(tool *aitool.Tool, params aitool.InvokeParams, reviewID string, outputChan chan *ypb.AIOutputEvent) {
	// Create review information
	reviewInfo := map[string]interface{}{
		"id":               reviewID,
		"selectors":        aid.ToolUseReviewSuggestions, // Use aid's review suggestions
		"tool":             tool.Name,
		"tool_description": tool.Description,
		"params":           params,
	}

	// Emit as structured event
	if outputChan != nil {
		content := fmt.Sprintf("Tool use review required for: %s\nTool Description: %s\nParameters: %v\nReview ID: %s",
			tool.Name, tool.Description, params, reviewID)

		event := &ypb.AIOutputEvent{
			Type:      "tool_use_review_require",
			Content:   []byte(content),
			IsJson:    false,
			IsSystem:  true,
			Timestamp: time.Now().Unix(),
		}

		select {
		case outputChan <- event:
		case <-r.config.ctx.Done():
			log.Warnf("Failed to emit tool use review event due to context cancellation")
		}
	}

	// Log the review requirement
	log.Infof("Tool use review required for tool: %s with params: %v", tool.Name, reviewInfo)
}
