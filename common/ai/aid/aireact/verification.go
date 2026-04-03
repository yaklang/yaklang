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

	enhanceData := r.DumpCurrentEnhanceData()
	var enhanceDataList []string
	if enhanceData != "" {
		enhanceDataList = []string{enhanceData}
	}
	verificationPrompt, nonce := r.generateVerificationPrompt(
		originalQuery, isToolCall, payload, enhanceDataList...,
	)
	if r.config.DebugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	result := &aicommon.VerifySatisfactionResult{}
	var referenceAnchorOnce sync.Once
	var referenceAnchorID string
	promptFallback := r.promptManager.GenerateVerificationPromptFallback(
		originalQuery,
		isToolCall,
		payload,
		nonce,
		enhanceDataList...,
	)

	captureReferenceAnchor := func(event *schema.AiOutputEvent) {
		if event == nil {
			return
		}
		streamID := event.GetStreamEventWriterId()
		if streamID == "" {
			log.Errorf("empty streamId provided for verification reference anchor, origin data: %v", string(event.Content))
			return
		}
		referenceAnchorOnce.Do(func() {
			referenceAnchorID = streamID
		})
	}

	emitVerificationReferenceMaterials := func(rawResponse string) {
		if strings.TrimSpace(referenceAnchorID) == "" {
			log.Warnf("skip verification reference materials because no stream anchor was emitted")
			return
		}
		aicommon.EmitAIRequestAndResponseReferenceMaterials(r.Emitter, referenceAnchorID, verificationPrompt, rawResponse)
	}

	log.Infof("Verifying if user needs are satisfied and formatting results...")
	transErr := aicommon.CallAITransaction(
		r.config, verificationPrompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("re-act-verify", true, r.Emitter)

			var rawResponse bytes.Buffer
			stream = io.TeeReader(stream, &rawResponse)

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
							}
						},
					)
					if err != nil {
						log.Errorf("failed to emit %s stream event: %v", key, err)
						return
					}
					captureReferenceAnchor(event)
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
								}
							},
						)
						if err != nil {
							log.Errorf("failed to emit human_readable_result stream event: %v", err)
							return
						}
						captureReferenceAnchor(event)
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
							func() {},
						)
						if err != nil {
							log.Errorf("failed to emit next_movements stream event: %v", err)
							return
						}
						captureReferenceAnchor(event)
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
			result.OutputFiles = action.GetStringSlice("output_files")

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

			markdownSnapshot := r.RenderVerificationTodoMarkdownSnapshot(result)
			if strings.TrimSpace(markdownSnapshot) != "" {
				var out bytes.Buffer
				var outputReader = io.TeeReader(strings.NewReader(markdownSnapshot), &out)
				var event *schema.AiOutputEvent
				event, err = r.Emitter.EmitTextMarkdownStreamEvent(
					"next_movements_snapshot",
					outputReader,
					taskID,
					func() {},
				)
				if err != nil {
					return utils.Errorf("failed to emit next_movements snapshot markdown stream event: %v", err)
				}
				captureReferenceAnchor(event)
			}

			deliveryFilesMarkdown := r.RenderVerificationOutputFilesMarkdown(result.OutputFiles)
			if strings.TrimSpace(deliveryFilesMarkdown) != "" {
				var out bytes.Buffer
				var outputReader = io.TeeReader(strings.NewReader(deliveryFilesMarkdown), &out)
				var event *schema.AiOutputEvent
				event, err = r.Emitter.EmitTextMarkdownStreamEvent(
					"delivery_files_snapshot",
					outputReader,
					taskID,
					func() {
						if out.Len() > 0 {
							r.AddToTimeline("delivery_files", out.String())
						}
					},
				)
				if err != nil {
					return utils.Errorf("failed to emit delivery files markdown stream event: %v", err)
				}
				captureReferenceAnchor(event)
				r.EmitFileArtifactWithExt("delivery_files", ".md", deliveryFilesMarkdown)
			}

			emitVerificationReferenceMaterials(rawResponse.String())
			return nil
		},
		aicommon.WithAIRequest_PromptFallback(promptFallback),
		aicommon.WithAIRequest_Source("verify_satisfaction"),
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
	case "doing", "pending":
		if id == "" {
			return ""
		}
		if content == "" {
			return fmt.Sprintf("- [DOING]: [id: %s]", id)
		}
		return fmt.Sprintf("- [DOING]: [id: %s]: %s", id, content)
	case "done":
		if id == "" {
			return ""
		}
		return fmt.Sprintf("- [x]: [id: %s]", id)
	case "delete":
		if id == "" {
			return ""
		}
		if content == "" {
			return fmt.Sprintf("- [DELETED]: [id: %s]", id)
		}
		return fmt.Sprintf("- [DELETED]: [id: %s]: %s", id, content)
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
		op := strings.ToLower(strings.TrimSpace(movement.GetString("op")))
		if op == "pending" {
			op = "doing"
		}
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
