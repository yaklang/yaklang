package aireact

import (
	"bytes"
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"io"
)

func (r *ReAct) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
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
						r.Emitter.EmitTextMarkdownStreamEvent(
							"re-act-loop-answer-payload",
							reader,
							rsp.GetTaskIndex(),
							func() {
								r.EmitResultAfterStream(out.String())
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
