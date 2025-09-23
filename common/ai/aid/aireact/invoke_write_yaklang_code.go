package aireact

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) invokeWriteYaklangCode(ctx context.Context, approach string) error {
	// start to write code:
	// * (optional)query code snippets
	// * write to file
	// * check syntax and lint
	// * (optional) run test cases
	// * (query document && modify to file)
	// * return

	satisfied := false
	iterationCount := 0
	maxIterations := 10
	currentCode := ""
	errorMessages := ""
	lastAction := ""

	userQuery := ""
	if r.config.memory != nil {
		userQuery = r.config.memory.Query
	}

	// Get available tools
	tools := buildinaitools.GetAllTools()

	for !satisfied && iterationCount < maxIterations {
		iterationCount++

		prompt, err := r.promptManager.GenerateYaklangCodeActionLoop(
			userQuery,      // userQuery
			currentCode,    // currentCode
			errorMessages,  // errorMessages
			lastAction,     // lastAction
			iterationCount, // iterationCount
			maxIterations,  // maxIterations
			tools,          // tools
		)
		if err != nil {
			return err
		}

		// Use aid.CallAITransaction for robust AI calling
		var action *aicommon.WaitableAction
		var nextAction aitool.InvokeParams
		var actionErr error

		transactionErr := aicommon.CallAITransaction(
			r.config, prompt, r.config.CallAI,
			func(resp *aicommon.AIResponse) error {
				stream := resp.GetOutputStreamReader("yaklang-code-loop", true, r.config.Emitter)
				subCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				action, actionErr = aicommon.ExtractWaitableActionFromStream(
					subCtx,
					stream,
					"action",
					[]string{},
					[]jsonextractor.CallbackOption{})
				if actionErr != nil {
					return utils.Errorf("Failed to parse action: %v", actionErr)
				}

				nextAction = action.WaitObject("action")
				actionType := nextAction.GetString("type")
				if actionType == "" {
					return utils.Errorf("Invalid action type: %s", actionType)
				}

				return nil
			})

		if transactionErr != nil {
			log.Errorf("AI transaction failed in code generation loop: %v", transactionErr)
			errorMessages = transactionErr.Error()
			lastAction = "ai_transaction_failed"
			continue
		}

		r.PushCumulativeSummaryHandle(func() string {
			return "Code generation iteration completed"
		})

		actionType := nextAction.GetString("type")
		reasoning := nextAction.GetString("reasoning")
		codeUpdate := nextAction.GetString("code_update")
		satisfied = nextAction.GetBool("satisfied")

		log.Infof("Code generation loop iteration %d: action=%s, satisfied=%v", iterationCount, actionType, satisfied)

		// Update state for next iteration
		if codeUpdate != "" {
			currentCode = codeUpdate
		}
		lastAction = actionType
		errorMessages = "" // Clear previous errors if action succeeded

		// Handle different action types
		switch actionType {
		case "query_code_snippets":
			// TODO: Implement code snippets querying
			r.addToTimeline("code_action", fmt.Sprintf("Querying code snippets: %s", reasoning))
		case "write_to_file":
			// TODO: Implement file writing
			r.addToTimeline("code_action", fmt.Sprintf("Writing to file: %s", reasoning))
		case "check_syntax_and_lint":
			// TODO: Implement syntax checking
			r.addToTimeline("code_action", fmt.Sprintf("Checking syntax: %s", reasoning))
		case "run_test_cases":
			// TODO: Implement test running
			r.addToTimeline("code_action", fmt.Sprintf("Running tests: %s", reasoning))
		case "query_document":
			// TODO: Implement document querying
			r.addToTimeline("code_action", fmt.Sprintf("Querying documentation: %s", reasoning))
		case "modify_file":
			// TODO: Implement file modification
			r.addToTimeline("code_action", fmt.Sprintf("Modifying file: %s", reasoning))
		default:
			log.Warnf("Unknown code action type: %s", actionType)
			r.addToTimeline("code_action", fmt.Sprintf("Unknown action: %s", actionType))
		}

		// If satisfied, break the loop
		if satisfied {
			r.addToTimeline("code_generation", "Code generation completed successfully")
			break
		}
	}

	return nil
}
