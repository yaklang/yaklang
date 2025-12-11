package aireact

import (
	"bytes"
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"io"
)

// DirectlyAnswerOption configures DirectlyAnswer behavior
type DirectlyAnswerOption func(*directlyAnswerConfig)

type directlyAnswerConfig struct {
	referenceMaterial       string
	referenceMaterialIdx    int
	skipEmitResultAfterDone bool // If true, skip emitting result after stream is done
}

// WithReferenceMaterial sets reference material to emit with the stream output
func WithReferenceMaterial(material string, idx int) DirectlyAnswerOption {
	return func(c *directlyAnswerConfig) {
		c.referenceMaterial = material
		c.referenceMaterialIdx = idx
	}
}

// WithSkipEmitResult skips emitting result after stream is done
// Use this when the caller will emit the result themselves
func WithSkipEmitResult() DirectlyAnswerOption {
	return func(c *directlyAnswerConfig) {
		c.skipEmitResultAfterDone = true
	}
}

func (r *ReAct) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	// Apply options
	config := &directlyAnswerConfig{}
	for _, opt := range opts {
		if fn, ok := opt.(DirectlyAnswerOption); ok {
			fn(config)
		}
	}

	// Check context cancellation early
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	prompt, nonceStr, err := r.promptManager.GenerateDirectlyAnswerPrompt(
		query,
		tools,
	)
	if err != nil {
		return "", err
	}

	var finalResult string
	var referenceOnce = new(sync.Once)

	// Helper to emit reference material when stream event is emitted
	emitReferenceMaterial := func(event *schema.AiOutputEvent) {
		if config.referenceMaterial == "" {
			return
		}
		streamId := event.GetContentJSONPath(`$.event_writer_id`)
		if streamId == "" {
			return
		}
		referenceOnce.Do(func() {
			taskIndex := ""
			if r.GetCurrentTask() != nil {
				taskIndex = r.GetCurrentTask().GetIndex()
			}
			// Get workdir from config
			workdir := r.config.Workdir
			// Emit reference material with file
			r.Emitter.EmitTextReferenceMaterialWithFile(
				streamId,
				config.referenceMaterial,
				workdir,
				taskIndex,
				config.referenceMaterialIdx,
			)
		})
	}

	err = aicommon.CallAITransaction(
		r.config,
		prompt,
		r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("directly_answer", true, r.Emitter)
			action, err := aicommon.ExtractActionFromStream(
				ctx,
				stream, "object",
				aicommon.WithActionNonce(nonceStr),
				aicommon.WithActionTagToKey("FINAL_ANSWER", "answer_payload"),
				aicommon.WithActionAlias("directly_answer"),
			aicommon.WithActionFieldStreamHandler(
				[]string{"answer_payload"},
				func(key string, reader io.Reader) {
					var out bytes.Buffer
					reader = utils.JSONStringReader(reader)
					reader = io.TeeReader(reader, &out)
					var event *schema.AiOutputEvent
					event, _ = r.Emitter.EmitTextMarkdownStreamEvent(
						"re-act-loop-answer-payload",
						reader,
						rsp.GetTaskIndex(),
						func() {
							// Only emit result if not skipped (caller will handle it)
							if !config.skipEmitResultAfterDone {
								r.EmitResultAfterStream(out.String())
							}
							if event != nil {
								emitReferenceMaterial(event)
							}
						},
					)
				}),
			)
			if err != nil {
				return err
			}
			var payload string
			if r := action.GetString("answer_payload"); r != "" {
				payload = r
			}
			if payload == "" {
				payload = action.GetString("next_action.answer_payload")
			}
			finalResult = payload
			return nil
		},
	)
	if finalResult != "" {
		return finalResult, nil
	}
	return finalResult, err
}
