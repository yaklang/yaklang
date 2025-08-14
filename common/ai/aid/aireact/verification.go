package aireact

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// verifyUserSatisfaction verifies if the tool execution satisfied the user's needs and provides human-readable output
func (r *ReAct) verifyUserSatisfaction(originalQuery, toolName string) (bool, string, error) {
	verificationPrompt := r.generateVerificationPrompt(originalQuery, toolName)

	if r.config.debugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	var satisfied bool
	var finalResult string
	var verificationErr error

	log.Infof("Verifying if user needs are satisfied and formatting results...")

	// Use the unified AI call wrapper instead of aid.CallAITransaction to ensure consistency
	resp, err := r.config.CallAI(verificationPrompt)
	if err != nil {
		return false, "", utils.Errorf("failed to call AI for verification: %v", err)
	}

	// Extract verification response
	responseContent := r.extractResponseContent(resp)

	// Parse verification result
	satisfied, finalResult, verificationErr = r.parseVerificationResponse(responseContent)
	if verificationErr != nil {
		return false, "", utils.Errorf("failed to parse verification response: %v", verificationErr)
	}

	return satisfied, finalResult, nil
}

// generateVerificationPrompt generates a prompt for verifying user satisfaction
func (r *ReAct) generateVerificationPrompt(originalQuery, toolName string) string {
	// Use the prompt manager to generate the prompt
	prompt, err := r.promptManager.GenerateVerificationPrompt(originalQuery, toolName)
	if err != nil {
		// Fallback to basic prompt if template fails
		log.Errorf("Failed to generate verification prompt from template: %v", err)
		return "Verify if the tool execution satisfied the user request."
	}
	return prompt
}

// parseVerificationResponse parses the verification response to extract satisfaction status and human-readable result
func (r *ReAct) parseVerificationResponse(response string) (bool, string, error) {
	// Try to extract @action first for validation
	action, err := aid.ExtractAction(response, "verify-satisfaction")
	if err == nil {
		// If we found a proper @action structure, extract data from it
		satisfied := action.GetBool("user_satisfied")
		result := action.GetString("human_readable_result")
		reasoning := action.GetString("reasoning")

		if r.config.debugEvent {
			log.Infof("Verification reasoning: %s", reasoning)
		}

		return satisfied, result, nil
	}

	// Fallback to JSON extraction if @action parsing fails
	if r.config.debugEvent {
		log.Warnf("Failed to parse verification as @action, falling back to JSON extraction: %v", err)
	}

	// Try direct JSON parsing
	var verificationResult struct {
		UserSatisfied       bool   `json:"user_satisfied"`
		HumanReadableResult string `json:"human_readable_result"`
		Reasoning           string `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(response), &verificationResult); err == nil {
		if r.config.debugEvent {
			log.Infof("Verification reasoning: %s", verificationResult.Reasoning)
		}
		return verificationResult.UserSatisfied, verificationResult.HumanReadableResult, nil
	}

	// Try JSON extractor as final fallback
	results := jsonextractor.ExtractStandardJSON(response)
	for _, result := range results {
		var verificationData struct {
			UserSatisfied       bool   `json:"user_satisfied"`
			HumanReadableResult string `json:"human_readable_result"`
			Reasoning           string `json:"reasoning"`
		}

		if err := json.Unmarshal([]byte(result), &verificationData); err == nil {
			if r.config.debugEvent {
				log.Infof("Verification reasoning: %s", verificationData.Reasoning)
			}
			return verificationData.UserSatisfied, verificationData.HumanReadableResult, nil
		}
	}

	// If all parsing methods fail, assume not satisfied and return raw response
	log.Warnf("Failed to parse verification response properly, assuming not satisfied")
	return false, strings.TrimSpace(response), nil
}
