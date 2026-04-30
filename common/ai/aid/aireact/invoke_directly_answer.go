package aireact

import (
	"bytes"
	"context"
	"sync"

	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// DirectlyAnswerOption is a deprecated alias kept for backward compatibility.
// 新代码请使用 aicommon.DirectlyAnswerOption 与 aicommon.WithDirectlyAnswerReferenceMaterial / aicommon.WithDirectlyAnswerSkipEmitResult。
//
// Deprecated: use aicommon.DirectlyAnswerOption.
type DirectlyAnswerOption = aicommon.DirectlyAnswerOption

// WithReferenceMaterial 是 aicommon.WithDirectlyAnswerReferenceMaterial 的别名，仅为兼容旧调用方而保留。
//
// Deprecated: use aicommon.WithDirectlyAnswerReferenceMaterial.
func WithReferenceMaterial(material string, idx int) DirectlyAnswerOption {
	return aicommon.WithDirectlyAnswerReferenceMaterial(material, idx)
}

// WithSkipEmitResult 是 aicommon.WithDirectlyAnswerSkipEmitResult 的别名，仅为兼容旧调用方而保留。
//
// Deprecated: use aicommon.WithDirectlyAnswerSkipEmitResult.
func WithSkipEmitResult() DirectlyAnswerOption {
	return aicommon.WithDirectlyAnswerSkipEmitResult()
}

func (r *ReAct) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	config := aicommon.ApplyDirectlyAnswerOptions(opts)

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
	errorWarp := func(err error) error {
		if err == nil {
			return nil
		}
		return utils.Wrapf(
			err,
			"AITAG retry hint: previous response format was invalid. If your final answer is long, multi-line, markdown, or code, you MUST use AITAG instead of answer_payload. Example:\n{\"@action\":\"directly_answer\"}\n<|FINAL_ANSWER_%s|>\n# your markdown answer\n<|FINAL_ANSWER_END_%s|>",
			nonceStr,
			nonceStr,
		)
	}

	var finalResult string
	var referenceOnce = new(sync.Once)

	emitReferenceMaterial := func(event *schema.AiOutputEvent) {
		if config.ReferenceMaterial == "" {
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
			workdir := r.config.Workdir
			r.Emitter.EmitTextReferenceMaterialWithFile(
				streamId,
				config.ReferenceMaterial,
				workdir,
				taskIndex,
				config.ReferenceMaterialIdx,
			)
		})
	}

	err = aicommon.CallAITransaction(
		r.config,
		prompt,
		r.config.CallQualityPriorityAI,
		func(rsp *aicommon.AIResponse) error {
			boundEmitter := rsp.BindEmitter(r.Emitter)
			stream := rsp.GetOutputStreamReader("directly_answer", true, r.Emitter)

			hasAnswerPayloadKey := false
			action, err := aicommon.ExtractActionFromStream(
				ctx,
				stream, "object",
				aicommon.WithActionNonce(nonceStr),
				aicommon.WithActionTagToKey("FINAL_ANSWER", "answer_payload"),
				aicommon.WithActionAlias("directly_answer"),
				aicommon.WithActionFieldStreamHandler(
					[]string{"answer_payload"},
					func(key string, reader io.Reader) {
						hasAnswerPayloadKey = true
						var out bytes.Buffer
						reader = utils.JSONStringReader(reader)
						reader = io.TeeReader(reader, &out)
						var event *schema.AiOutputEvent
						event, _ = boundEmitter.EmitTextMarkdownStreamEvent(
							"re-act-loop-answer-payload",
							reader,
							rsp.GetTaskIndex(),
							func() {
								if !config.SkipEmitResultAfterDone {
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
				return errorWarp(err)
			}
			action.WaitStream(ctx)
			var payload string
			if r := action.GetString("answer_payload"); r != "" {
				payload = r
			}
			if payload == "" {
				payload = action.GetString("next_action.answer_payload")
			}
			if payload == "" && !hasAnswerPayloadKey {
				return errorWarp(utils.Error("no answer_payload key in stream"))
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
