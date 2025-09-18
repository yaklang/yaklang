package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// verifyUserSatisfaction verifies if the materials satisfied the user's needs and provides human-readable output
func (r *ReAct) verifyUserSatisfaction(originalQuery string, isToolCall bool, payload string) (bool, string, error) {
	verificationPrompt := r.generateVerificationPrompt(
		originalQuery, isToolCall, payload, r.GetCurrentTask().DumpEnhanceData(),
	)
	if r.config.debugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	var satisfied bool
	var result string
	var reason string

	log.Infof("Verifying if user needs are satisfied and formatting results...")
	transErr := aicommon.CallAITransaction(
		r.config, verificationPrompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("re-act-verify", false, r.Emitter)
			action, err := aicommon.ExtractActionFromStream(stream, "verify-satisfaction")
			if err != nil {
				return utils.Errorf("failed to extract verification action: %v, need ...\"@action\":\"verify-satisfaction\" ", err)
			}
			// If we found a proper @action structure, extract data from it
			satisfied = action.GetBool("user_satisfied")
			result = action.GetString("human_readable_result")
			reason = action.GetString("reasoning")

			if result == "" && reason == "" {
				return utils.Error("both human_readable_result and reasoning are empty, at least one must be provided")
			}
			return nil
		},
	)
	if transErr != nil {
		log.Errorf("AI transaction failed during verification: %v", transErr)
		return false, "", transErr
	}

	var finalResult string
	if result != "" {
		finalResult = result
	}
	if reason != "" {
		finalResult += "\nReasoning: " + reason
	}

	return satisfied, finalResult, nil
}

// generateVerificationPrompt generates a prompt for verifying user satisfaction
func (r *ReAct) generateVerificationPrompt(originalQuery string, isToolCall bool, payload string, enhanceData ...string) string {
	// Use the prompt manager to generate the prompt
	prompt, err := r.promptManager.GenerateVerificationPrompt(originalQuery, isToolCall, payload, enhanceData...)
	if err != nil {
		// Fallback to basic prompt if template fails
		log.Errorf("Failed to generate verification prompt from template: %v", err)
		return "Verify if the tool execution satisfied the user request."
	}
	return prompt
}
