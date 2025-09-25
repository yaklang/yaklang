package aireact

import (
	"bytes"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// verifyUserSatisfaction verifies if the materials satisfied the user's needs and provides human-readable output
func (r *ReAct) verifyUserSatisfaction(originalQuery string, isToolCall bool, payload string) (bool, error) {
	verificationPrompt := r.generateVerificationPrompt(
		originalQuery, isToolCall, payload, r.DumpCurrentEnhanceData(),
	)
	if r.config.debugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	var satisfied bool
	log.Infof("Verifying if user needs are satisfied and formatting results...")
	transErr := aicommon.CallAITransaction(
		r.config, verificationPrompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("re-act-verify", true, r.Emitter)

			createReasonCallback := func(prompt string) func(key string, reader io.Reader, parents []string) {
				return func(key string, reader io.Reader, parents []string) {
					var out bytes.Buffer
					reader = io.TeeReader(utils.JSONStringReader(utils.UTF8Reader(reader)), &out)
					r.Emitter.EmitStreamEvent(
						"re-act-verify",
						time.Now(),
						reader,
						rsp.GetTaskIndex(),
						func() {
							if out.Len() > 0 {
								r.addToTimeline("verify", prompt+": "+out.String())
							}
						},
					)
				}
			}

			action, err := aicommon.ExtractWaitableActionFromStream(
				r.config.GetContext(),
				stream, "verify-satisfaction", []string{}, []jsonextractor.CallbackOption{
					jsonextractor.WithRegisterFieldStreamHandler("human_readable_result", createReasonCallback("Result")),
					jsonextractor.WithRegisterFieldStreamHandler("reasoning", createReasonCallback("Reasoning")),
				})
			if err != nil {
				return utils.Errorf("failed to extract verification action: %v, need ...\"@action\":\"verify-satisfaction\" ", err)
			}
			// If we found a proper @action structure, extract data from it
			satisfied = action.WaitBool("user_satisfied")
			return nil
		},
	)
	if transErr != nil {
		log.Errorf("AI transaction failed during verification: %v", transErr)
		return false, transErr
	}
	return satisfied, nil
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
