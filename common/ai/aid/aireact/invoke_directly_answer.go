package aireact

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	var aiTagResult string
	var wg = new(sync.WaitGroup)
	err = aicommon.CallAITransaction(
		r.config,
		prompt,
		r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("directly_answer", true, r.Emitter)
			wg.Add(1)
			stream = utils.CreateUTF8StreamMirror(stream, func(reader io.Reader) {
				defer func() {
					wg.Done()
				}()
				err := aitag.Parse(
					utils.UTF8Reader(reader),
					aitag.WithCallback("FINAL_ANSWER", nonceStr, func(rd io.Reader) {
						var out bytes.Buffer
						r.Emitter.EmitTextMarkdownStreamEvent(
							"re-act-loop-answer-payload",
							io.TeeReader(rd, &out),
							rsp.GetTaskIndex(),
							func() {
								aiTagResult = out.String()
								r.EmitResultAfterStream(out.String())
							},
						)
					}))
				if err != nil && err != io.EOF {
					log.Warnf("DirectlyAnswer failed: %v", err)
				}
			})

			action, err := aicommon.ExtractActionFromStreamWithJSONExtractOptions(
				stream, "object", []string{
					"directly_answer",
				}, []jsonextractor.CallbackOption{
					jsonextractor.WithRegisterFieldStreamHandler(
						"answer_payload",
						func(key string, reader io.Reader, parents []string) {
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
						},
					),
				})
			if err != nil {
				return err
			}
			var payload string
			if r := action.GetString("answer_payload"); r != "" {
				payload = r
			}
			if payload == "" {
				payload = action.GetInvokeParams("next_action").GetString("answer_payload")
			}
			finalResult = payload
			return nil
		},
	)
	wg.Wait()
	if aiTagResult != "" {
		return aiTagResult, nil
	}
	if finalResult != "" {
		return finalResult, nil
	}
	return finalResult, err
}
