package aireact

import (
	"bytes"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) DirectlyAnswer(query string, tools []*aitool.Tool) (string, error) {
	prompt, err := r.promptManager.GenerateDirectlyAnswerPrompt(
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
			action, err := aicommon.ExtractActionFromStreamWithJSONExtractOptions(
				stream, "object", []string{}, []jsonextractor.CallbackOption{
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
			result := action.GetInvokeParams("next_action").GetString("answer_payload")
			if result != "" {
				finalResult = result
				return nil
			}
			return utils.Error("answer_payload is required but empty in action")
		},
	)
	return finalResult, err
}
