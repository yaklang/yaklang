package aireact

import (
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
	// Use the prompt manager to generate the prompt
	prompt, err := r.promptManager.GenerateToolParamsPrompt(tool)
	if err != nil {
		// Fallback to basic prompt if template fails
		log.Errorf("Failed to generate tool params prompt from template: %v", err)
		return fmt.Sprintf("Generate parameters for tool '%s': %s", tool.Name, tool.Description)
	}
	return prompt
}

// extractOriginalUserQuery extracts the original user query from memory (thread-safe)
func (r *ReAct) extractOriginalUserQuery() string {
	r.config.mu.RLock()
	defer r.config.mu.RUnlock()
	return r.config.memory.Query
}
