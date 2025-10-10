package aireact

import (
	"bytes"
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) EnhanceKnowledgeAnswer(ctx context.Context, userQuery string) (string, error) {
	currentTask := r.GetCurrentTask()
	enhanceID := uuid.NewString()
	config := r.config

	if config.enhanceKnowledgeManager == nil {
		return "", utils.Errorf("enhanceKnowledgeManager is not configured, but ai choice knowledge enhance answer action, check main loop prompt!")
	}

	enhanceData, err := config.enhanceKnowledgeManager.FetchKnowledge(r.config.ctx, userQuery)
	if err != nil {
		return "", utils.Errorf("enhanceKnowledgeManager.FetchKnowledge(%s) failed: %v", userQuery, err)
	}

	for enhanceDatum := range enhanceData {
		r.EmitKnowledge(enhanceID, enhanceDatum)
		config.enhanceKnowledgeManager.AppendKnowledge(currentTask.GetId(), enhanceDatum)
	}

	queryPrompt, err := r.promptManager.GenerateDirectlyAnswerPrompt(userQuery, nil, r.DumpCurrentEnhanceData())
	if err != nil {
		return "", err
	}

	var finalResult string
	err = aicommon.CallAITransaction(
		r.config,
		queryPrompt,
		r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("directly_answer", true, r.Emitter)
			subCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			waitAction, err := aicommon.ExtractWaitableActionFromStream(
				subCtx,
				stream, "object", []string{},
				[]jsonextractor.CallbackOption{
					jsonextractor.WithRegisterFieldStreamHandler(
						"answer_payload",
						func(key string, reader io.Reader, parents []string) {
							var output bytes.Buffer
							reader = utils.JSONStringReader(utils.UTF8Reader(reader))
							reader = io.TeeReader(reader, &output)
							r.config.Emitter.EmitTextMarkdownStreamEvent(
								"re-act-loop-answer-payload",
								reader,
								rsp.GetTaskIndex(),
								func() {
									r.EmitResultAfterStream(output.String())
								},
							)
						},
					),
				})
			if err != nil {
				return err
			}
			nextAction := waitAction.WaitObject("next_action") // ensure next_action is fully received
			if nextAction == nil || nextAction.GetString("answer_payload") == "" {
				return utils.Error("answer_payload is required but empty in action")
			}
			finalResult = nextAction.GetString("answer_payload")
			return nil
		},
	)
	r.EmitTextArtifact("enhance_directly_answer", finalResult)
	return finalResult, err
}
