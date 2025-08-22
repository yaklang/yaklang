package aireact

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
func (r *ReAct) executeMainLoop(userQuery string) error {
	if r.config.debugEvent {
		log.Infof("executeMainLoop started with query: %s", userQuery)
		log.Infof("ReAct AI Error Learning: 已启用增强的AI错误学习功能，AI将自动从失败中学习并改进响应质量")
	}

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
		// Set AI instance for newly created memory
		if r.config.aiCallback != nil {
			r.config.memory.SetTimelineAI(r.config)
		}
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
		// Acquire lock for this iteration
		// Check if finished while holding lock
		if r.config.finished {
			break
		}

		if r.config.debugEvent {
			log.Infof("executeMainLoop: entering loop iteration. currentIteration=%d, maxIterations=%d, finished=%t",
				r.config.currentIteration, r.config.maxIterations, r.config.finished)
		}
		r.config.currentIteration++

		if r.config.debugEvent {
			log.Infof("Starting ReAct iteration %d/%d", r.config.currentIteration, r.config.maxIterations)
		}
		r.EmitIteration(r.config.currentIteration, r.config.maxIterations)

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
		var action *aicommon.Action
		var actionErr error

		// Temporarily release lock for AI transaction to prevent deadlocks
		transactionErr := aicommon.CallAITransaction(r.config, prompt,
			func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				// Use the stored callback
				return r.config.aiCallback(r.config, req)
			},
			func(resp *aicommon.AIResponse) error {
				// Extract response content
				responseContent := r.extractResponseContent(resp)

				if r.config.debugEvent {
					log.Infof("Attempting to parse response: %s", responseContent)
				}

				// Parse action from response using aicommon.ExtractAction
				action, actionErr = r.parseReActAction(responseContent)
				if actionErr != nil {
					return utils.Errorf("Failed to parse action: %v", actionErr)
				}

				return nil
			})

		if transactionErr != nil {
			log.Errorf("AI transaction failed (内置错误学习功能): %v", transactionErr)
			continue
		}

		if actionErr != nil {
			log.Errorf("Failed to parse action: %v", actionErr)
			continue
		}

		if r.config.debugEvent {
			actionType := action.GetInvokeParams("next_action").GetString("type")
			thought := action.GetString("human_readable_thought")
			log.Infof("Parsed action: type=%s, thought=%s", actionType, thought)
		}

		// Emit human readable thought
		r.EmitThought(action.GetString("human_readable_thought"))

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
			log.Infof("Providing direct answer")
			answerPayload := action.GetInvokeParams("next_action").GetString("answer_payload")
			r.EmitAction(fmt.Sprintf("Answer: %s", answerPayload))
			r.EmitResult(answerPayload)
			// Always mark as finished for direct answers to avoid loops
			r.config.finished = true
			// Store interaction in memory (no tool call result for direct answers)
		case ActionRequireTool:
			toolPayload := action.GetInvokeParams("next_action").GetString("tool_require_payload")
			log.Infof("Requesting tool: %s", toolPayload)
			if err := r.handleRequireTool(toolPayload); err != nil {
				log.Errorf("Tool execution failed: %v", err)
			} else {
				// Tool executed successfully, now verify if user needs are satisfied
				// Temporarily release the lock before calling verification to avoid deadlock
				satisfied, finalResult, err := r.verifyUserSatisfaction(userQuery, toolPayload)
				if err != nil {
					log.Errorf("Verification failed: %v", err)
					r.config.finished = true
				} else if satisfied {
					// User needs are satisfied, emit final result and finish
					r.EmitResult(finalResult)
					r.config.finished = true
				} else {
					// User needs not satisfied, continue loop
					log.Infof("User needs not fully satisfied, continuing analysis...")
				}
			}
		case ActionRequestPlanExecution:
			planPayload := action.GetInvokeParams("next_action").GetString("plan_request_payload")
			log.Infof("Requesting plan execution: %s, start to create p-e coordinator", planPayload)
			if err := r.invokePlanAndExecute(planPayload); err != nil {
				log.Errorf("Plan execution failed: %v", err)
			}
		case ActionAskForClarification:
			nextAction := action.GetInvokeParams("next_action")
			obj := nextAction.GetObject("ask_for_clarification_payload")
			payloads := obj.GetStringSlice("options")
			question := obj.GetString("question")
			ep := r.config.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE)
			ep.SetDefaultSuggestionContinue()
			var opts []map[string]any
			for i, payload := range payloads {
				opts = append(opts, map[string]any{
					"index":        i + 1,
					"prompt_title": payload,
				})
			}
			result := map[string]any{
				"id":      ep.GetId(),
				"prompt":  question,
				"options": opts,
			}
			ep.SetReviewMaterials(result)
			err := r.config.SubmitCheckpointRequest(ep.GetCheckpoint(), result)
			if err != nil {
				log.Errorf("Failed to submit checkpoint request: %v", err)
			}
			r.config.EmitInteractiveJSON(
				ep.GetId(),
				schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE,
				"require-user-interact",
				result,
			)
			ctx := r.config.GetContext()
			ctx = utils.SetContextKey(ctx, SKIP_AI_REVIEW, true)
			r.config.DoWaitAgree(ctx, ep)
			params := ep.GetParams()
			r.config.EmitInteractiveRelease(ep.GetId(), params)
			r.config.CallAfterInteractiveEventReleased(ep.GetId(), params)
			r.addToTimeline(
				"ask_for_clarification",
				fmt.Sprintf("User clarification requested: %s result: %v",
					action.GetString("human_readable_thought"), spew.Sdump(params)),
				ep.GetId(),
			)
		default:
			r.EmitError("unknown action type: %v", actionType)
			r.config.finished = true
		}
	}
	if r.config.currentIteration >= r.config.maxIterations {
		r.EmitWarning("Too many iterations[%v] is reached, stopping ReAct loop, max: %v", r.config.currentIteration, r.config.maxIterations)
	}
	return nil
}
