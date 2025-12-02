package aireact

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"sync"
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

	verificationPrompt, nonce := r.generateVerificationPrompt(
		originalQuery, isToolCall, payload, r.DumpCurrentEnhanceData(),
	)
	if r.config.DebugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	var satisfied bool
	var reasoning string
	var referenceOnce = new(sync.Once)

	emitStreamIdReference := func(event *schema.AiOutputEvent) {
		streamId := event.GetContentJSONPath(`$.event_writer_id`)
		if streamId == "" {
			log.Errorf("empty streamId provided for reference emission, origin data: %v", string(event.Content))
			return
		}
		referenceOnce.Do(func() {
			_, _ = r.EmitTextReferenceMaterial(streamId, utils.MustRenderTemplate(`<|ORIGINAL_QUERY|>
{{ .OriginalQuery }}
<|ORIGINAL_QUERY_END|>

{{ if .IsToolCall }}<|IS_TOOL_CALL|>
{{ .Payload }}<|IS_TOOL_CALL_END|>
{{ else }}<|VERIFICATION_PAYLOAD|>
{{ .Payload }}
<|VERIFICATION_PAYLOAD_END|>
{{ end }}
`, map[string]any{
				"OriginalQuery": originalQuery,
				"IsToolCall":    isToolCall,
				"Payload":       payload,
			}))
		})
	}

	log.Infof("Verifying if user needs are satisfied and formatting results...")
	transErr := aicommon.CallAITransaction(
		r.config, verificationPrompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("re-act-verify", true, r.Emitter)

			// stream = io.TeeReader(stream, os.Stdout)

			createReasonCallback := func(prompt string) func(key string, reader io.Reader) {
				return func(key string, reader io.Reader) {
					var out bytes.Buffer
					reader = io.TeeReader(utils.JSONStringReader(utils.UTF8Reader(reader)), &out)
					var event *schema.AiOutputEvent
					var err error
					event, err = r.Emitter.EmitDefaultStreamEvent(
						"re-act-verify",
						reader,
						rsp.GetTaskIndex(),
						func() {
							if out.Len() > 0 {
								r.AddToTimeline("verify", prompt+": "+out.String())
								emitStreamIdReference(event)
							}
						},
					)
					if err != nil {
						log.Errorf("failed to emit %s stream event: %v", key, err)
						return
					}
				}
			}

			taskID := ""
			if r.GetCurrentTask() != nil {
				taskID = r.GetCurrentTask().GetId()
			}

			action, err := aicommon.ExtractValidActionFromStream(
				ctx,
				stream, "verify-satisfaction",
				aicommon.WithActionNonce(nonce),
				aicommon.WithActionTagToKey("HUMAN_READABLE_RESULT", "human_readable_result"),
				aicommon.WithActionFieldStreamHandler(
					[]string{"human_readable_result"},
					func(key string, read io.Reader) {
						var out bytes.Buffer
						var outputReader = io.TeeReader(utils.JSONStringReader(utils.UTF8Reader(read)), &out)
						var event *schema.AiOutputEvent
						var err error
						event, err = r.Emitter.EmitTextMarkdownStreamEvent(
							"human_readable_result", outputReader, taskID,
							func() {
								if out.Len() > 0 {
									r.AddToTimeline("human_readable_result", out.String())
									emitStreamIdReference(event)
								}
							},
						)
						if err != nil {
							log.Errorf("failed to emit human_readable_result stream event: %v", err)
							return
						}
						streamEventId := jsonpath.FindFirst(event.Content, `$.event_writer_id`) // stream event id != "
						streamId := utils.InterfaceToString(streamEventId)
						if streamId != "" {

						}
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
func (r *ReAct) generateVerificationPrompt(originalQuery string, isToolCall bool, payload string, enhanceData ...string) (string, string) {
	// Use the prompt manager to generate the prompt
	prompt, nonce, err := r.promptManager.GenerateVerificationPrompt(originalQuery, isToolCall, payload, enhanceData...)
	if err != nil {
		// Fallback to basic prompt if template fails
		log.Errorf("Failed to generate verification prompt from template: %v", err)
		return "Verify if the tool execution satisfied the user request.", nonce
	}
	return prompt, nonce
}
