package aid

import (
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"text/template"
	"time"
)

func CallAITransactionWithoutConfig(
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
) error {
	return CallAITransaction(nil, prompt, callAi, postHandler)
}

func CallAITransaction(
	c *Config,
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
) error {
	var seq int64
	var saver CheckpointCommitHandler
	var trcRetry int64 = 3
	if c != nil {
		trcRetry = c.aiTransactionAutoRetry
	}
	if trcRetry <= 0 {
		trcRetry = 3
	}
	var postHandlerErr error

	for i := int64(0); i < trcRetry; i++ {
		rsp, err := callAi(
			NewAIRequest(
				RetryPromptBuilder(prompt, postHandlerErr),
				WithAIRequest_SeqId(seq),
				WithAIRequest_OnAcquireSeq(func(i int64) {
					seq = i
				}),
				WithAIRequest_SaveCheckpointCallback(func(handler CheckpointCommitHandler) {
					saver = handler
				}),
			))
		if err != nil {
			c.EmitError("call ai api error: %v, retry and block it", err)
			select {
			case <-c.ctx.Done():
				return err
			case <-time.After(100 * time.Millisecond):
				c.EmitWarning("call ai transaction retry")
				continue
			}
		}
		postHandlerErr = postHandler(rsp)
		if postHandlerErr != nil {
			c.EmitError("ai transaction in postHandler error: %v, retry and block it", postHandlerErr)
			select {
			case <-c.ctx.Done():
				return postHandlerErr
			case <-time.After(100 * time.Millisecond):
				c.EmitWarning("call ai transaction retry")
				continue
			}
		}
		if saver != nil {
			cp, err := saver()
			if cp == nil {
				c.EmitError("cannot save checkpoint")
				return err
			} else {
				c.EmitInfo("checkpoint cached in database: %v:%v", utils.ShrinkString(cp.CoordinatorUuid, 12), cp.Seq)
			}
		}
		return nil
	}
	return utils.Errorf("max retry count[%v] reached in transaction", trcRetry)
}

func (c *Config) callAiTransaction(
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
) error {
	return CallAITransaction(c, prompt, callAi, postHandler)
}

var retryPromptTemplate = `
你现在正在处理一个任务，但由于某些原因需要重新开始。请根据以下信息回答：
# 重试原因
{{ .RetryReason }}

# 原始提示
{{ .RawPrompt }}

`

func RetryPromptBuilder(rawPrompt string, retryErr error) string {
	if retryErr == nil {
		return rawPrompt
	}
	templateData := map[string]interface{}{
		"RetryReason": rawPrompt,
		"RawPrompt":   retryErr.Error(),
	}
	tmpl, err := template.New("retry-prompt").Parse(retryPromptTemplate)
	if err != nil {
		return rawPrompt
	}
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, templateData)
	if err != nil {
		return rawPrompt
	}
	return promptBuilder.String()
}
