package aireact

import (
	"bytes"
	"context"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// VerifyUserSatisfaction verifies if the materials satisfied the user's needs and provides human-readable output
func (r *ReAct) VerifyUserSatisfaction(ctx context.Context, originalQuery string, isToolCall bool, payload string) (bool, string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}
	// Check context cancellation early
	select {
	case <-ctx.Done():
		return false, "", ctx.Err()
	default:
	}

	verificationPrompt := r.generateVerificationPrompt(
		originalQuery, isToolCall, payload, r.DumpCurrentEnhanceData(),
	)
	if r.config.DebugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	var satisfied bool
	var reasoning string
	log.Infof("Verifying if user needs are satisfied and formatting results...")
	transErr := aicommon.CallAITransaction(
		r.config, verificationPrompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("re-act-verify", true, r.Emitter)

			createReasonCallback := func(prompt string) func(key string, reader io.Reader) {
				return func(key string, reader io.Reader) {
					var out bytes.Buffer
					reader = io.TeeReader(utils.JSONStringReader(utils.UTF8Reader(reader)), &out)
					r.Emitter.EmitDefaultStreamEvent(
						"re-act-verify",
						reader,
						rsp.GetTaskIndex(),
						func() {
							if out.Len() > 0 {
								r.AddToTimeline("verify", prompt+": "+out.String())
							}
						},
					)
				}
			}

			taskID := ""
			if r.GetCurrentTask() != nil {
				taskID = r.GetCurrentTask().GetId()
			}
			action, err := aicommon.ExtractValidActionFromStream(
				ctx,
				stream, "verify-satisfaction",
				aicommon.WithActionFieldStreamHandler(
					[]string{"human_readable_result"},
					func(key string, read io.Reader) {
						r.Emitter.EmitThoughtStreamReader(taskID, read)
					},
				),
				aicommon.WithActionFieldStreamHandler(
					[]string{"reasoning"},
					createReasonCallback("Reasoning"),
				),
				aicommon.WithActionFieldStreamHandler(
					[]string{"next_movements"},
					func(key string, rd io.Reader) {
						r.Emitter.EmitDefaultStreamEvent(
							"next_movements",
							utils.JSONStringReader(rd),
							rsp.GetTaskIndex(),
						)
					},
				),
			)
			if err != nil {
				return utils.Errorf("failed to extract verification action: %v, need ...\"@action\":\"verify-satisfaction\" ", err)
			}
			// If we found a proper @action structure, extract data from it
			satisfied = action.GetBool("user_satisfied")
			reasoning = action.GetString("reasoning")

			nextMovements := action.GetString("next_movements") // currently not used
			if nextMovements != "" {
				r.AddToTimeline("next_movements", utils.MustRenderTemplate(`
<|NEXT_MOVEMENTS_{{.Nonce}}|>
{{ .NextMovements }}
<|NEXT_MOVEMENTS_END_{{.Nonce}}|>
`, map[string]string{
					"Nonce":         utils.RandStringBytes(4),
					"NextMovements": nextMovements,
				}))
			}
			return nil
		},
	)
	if transErr != nil {
		log.Errorf("AI transaction failed during verification: %v", transErr)
		return false, "", transErr
	}

	return satisfied, reasoning, nil
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
