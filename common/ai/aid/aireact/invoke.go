package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Schema and action types are now managed by the prompt manager

// generateMainLoopPrompt generates the prompt for the main ReAct loop
func (r *ReAct) generateMainLoopPrompt(userQuery string, tools []*aitool.Tool) string {
	// Use the prompt manager to generate the prompt
	prompt, err := r.promptManager.GenerateLoopPrompt(userQuery, tools)
	if err != nil {
		// Fallback to basic prompt if template fails
		log.Errorf("Failed to generate loop prompt from template: %v", err)
		return fmt.Sprintf("User Query: %s\nPlease respond with a JSON object for ReAct action.", userQuery)
	}
	return prompt
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
