package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// VerifyUserSatisfaction verifies if the materials satisfied the user's needs and provides human-readable output
func (r *ReAct) VerifyUserSatisfaction(ctx context.Context, originalQuery string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}
	// Check context cancellation early
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	verificationPrompt, nonce := r.generateVerificationPrompt(
		originalQuery, isToolCall, payload, r.DumpCurrentEnhanceData(),
	)
	if r.config.DebugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	result := &aicommon.VerifySatisfactionResult{}
	var referenceOnce = new(sync.Once)
	var nextMovementsReferenceOnce = new(sync.Once)

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

	emitNextMovementsReference := func(event *schema.AiOutputEvent) {
		streamId := event.GetContentJSONPath(`$.event_writer_id`)
		if streamId == "" {
			log.Errorf("empty streamId provided for next_movements reference emission, origin data: %v", string(event.Content))
			return
		}
		nextMovementsReferenceOnce.Do(func() {
			_, _ = r.EmitTextReferenceMaterial(streamId, verificationPrompt)
		})
	}

	log.Infof("Verifying if user needs are satisfied and formatting results...")
	transErr := aicommon.CallAITransaction(
		r.config, verificationPrompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("re-act-verify", true, r.Emitter)

			var rawResponse bytes.Buffer
			stream = io.TeeReader(stream, &rawResponse)

			var eventIds = new(sync.Map)

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
							eventIds.Store(event.GetStreamEventWriterId(), struct{}{})
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
						eventIds.Store(event.GetStreamEventWriterId(), struct{}{})
					},
				),
				aicommon.WithActionFieldStreamHandler(
					[]string{"reasoning"},
					createReasonCallback("Reasoning"),
				),
				aicommon.WithActionFieldStreamHandler(
					[]string{"next_movements"},
					func(key string, rd io.Reader) {
						trimmedReader := utils.NewTrimLeftReader(utils.UTF8Reader(rd))
						peekedReader := utils.NewPeekableReader(trimmedReader)
						firstByte, err := peekedReader.Peek(1)
						if err != nil && len(firstByte) == 0 {
							log.Infof("no next_movements provided in verification result, skipping next_movements stream handling")
							return
						}

						var displayReader io.Reader
						if len(firstByte) > 0 && firstByte[0] == '[' {
							pr, pw := utils.NewBufPipe(nil)
							go func() {
								defer pw.Close()
								if err := writeNextMovementsDisplayStream(peekedReader, pw); err != nil {
									log.Errorf("failed to stream next_movements display: %v", err)
								}
							}()
							displayReader = pr
						} else {
							displayReader = peekedReader
						}

						var out bytes.Buffer
						var outputReader = io.TeeReader(displayReader, &out)
						var event *schema.AiOutputEvent
						event, err = r.Emitter.EmitDefaultStreamEvent(
							"next_movements",
							outputReader,
							rsp.GetTaskIndex(),
							func() {
								if out.Len() > 0 {
									emitNextMovementsReference(event)
								}
							},
						)
						if err != nil {
							log.Errorf("failed to emit next_movements stream event: %v", err)
							return
						}
						if out.Len() == 0 {
							return
						}
						eventIds.Store(event.GetStreamEventWriterId(), struct{}{})
					},
				),
			)
			if err != nil {
				return utils.Errorf("failed to extract verification action: %v, need ...\"@action\":\"verify-satisfaction\" ", err)
			}
			// If we found a proper @action structure, extract data from it
			result.Satisfied = action.GetBool("user_satisfied")
			result.Reasoning = action.GetString("reasoning")
			result.CompletedTaskIndex = action.GetString("completed_task_index")

			nextMovements := normalizeVerifyNextMovements(action)
			// Store next_movements in result for status tracking
			result.NextMovements = nextMovements
			if len(nextMovements) > 0 {
				nextMovementsJSON, err := json.MarshalIndent(nextMovements, "", "  ")
				if err != nil {
					return utils.Errorf("failed to marshal next_movements: %v", err)
				}
				r.AddToTimeline("next_movements", utils.MustRenderTemplate(`
<|NEXT_MOVEMENTS_{{.Nonce}}|>
{{ .NextMovements }}
<|NEXT_MOVEMENTS_END_{{.Nonce}}|>
`, map[string]string{
					"Nonce":         utils.RandStringBytes(4),
					"NextMovements": string(nextMovementsJSON),
				}))
			}

			eventIds.Range(func(k, v interface{}) bool {
				kString := utils.InterfaceToString(k)
				r.EmitTextReferenceMaterial(kString, rawResponse.String())
				return true
			})
			return nil
		},
	)
	if transErr != nil {
		log.Errorf("AI transaction failed during verification: %v", transErr)
		return nil, transErr
	}
	r.AppendVerificationHistory(result)

	return result, nil
}

func writeNextMovementsDisplayStream(reader io.Reader, writer io.Writer) error {
	decoder := json.NewDecoder(reader)
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '[' {
		return utils.Errorf("next_movements is not a JSON array")
	}

	firstLine := true
	for decoder.More() {
		var movement aicommon.VerifyNextMovement
		if err := decoder.Decode(&movement); err != nil {
			return err
		}
		line := formatNextMovementDisplayLine(movement)
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !firstLine {
			if _, err := writer.Write([]byte("\n")); err != nil {
				return err
			}
		}
		firstLine = false
		if _, err := io.WriteString(writer, line); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func formatNextMovementDisplayLine(movement aicommon.VerifyNextMovement) string {
	id := strings.TrimSpace(movement.ID)
	content := strings.TrimSpace(movement.Content)
	switch strings.ToLower(strings.TrimSpace(movement.Op)) {
	case "add":
		if id == "" && content == "" {
			return ""
		}
		if id == "" {
			return fmt.Sprintf("- [+]: %s", content)
		}
		if content == "" {
			return fmt.Sprintf("- [+]: [id: %s]", id)
		}
		return fmt.Sprintf("- [+]: [id: %s]: %s", id, content)
	case "done":
		if id == "" {
			return ""
		}
		return fmt.Sprintf("- [x]: [id: %s]", id)
	default:
		label := strings.ToUpper(strings.TrimSpace(movement.Op))
		if label == "" {
			label = "?"
		}
		if id == "" && content == "" {
			return ""
		}
		if id == "" {
			return fmt.Sprintf("- [%s]: %s", label, content)
		}
		if content == "" {
			return fmt.Sprintf("- [%s]: [id: %s]", label, id)
		}
		return fmt.Sprintf("- [%s]: [id: %s]: %s", label, id, content)
	}
}

func normalizeVerifyNextMovements(action *aicommon.Action) []aicommon.VerifyNextMovement {
	if action == nil {
		return nil
	}
	nextMovementsRaw := action.GetInvokeParamsArray("next_movements")
	nextMovements := make([]aicommon.VerifyNextMovement, 0, len(nextMovementsRaw))
	for _, movement := range nextMovementsRaw {
		if movement == nil {
			continue
		}
		op := strings.TrimSpace(movement.GetString("op"))
		id := strings.TrimSpace(movement.GetString("id"))
		content := strings.TrimSpace(movement.GetString("content"))
		if op == "" || id == "" {
			continue
		}
		nextMovements = append(nextMovements, aicommon.VerifyNextMovement{
			Op:      op,
			Content: content,
			ID:      id,
		})
	}
	if len(nextMovements) > 0 {
		return nextMovements
	}

	legacy := strings.TrimSpace(action.GetString("next_movements"))
	if legacy == "" {
		return nil
	}
	return []aicommon.VerifyNextMovement{{
		Op:      "add",
		ID:      "legacy_next_movements",
		Content: legacy,
	}}
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
