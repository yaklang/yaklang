package aid

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func CallAITransactionWithoutConfig(
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
	requestOpts ...AIRequestOption,
) error {
	return CallAITransaction(nil, prompt, callAi, postHandler, requestOpts...)
}

func CallAITransaction(
	c *Config,
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
	requestOpts ...AIRequestOption,
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

	requestOpts = append(requestOpts,
		WithAIRequest_OnAcquireSeq(func(i int64) {
			seq = i
		}),
		WithAIRequest_SaveCheckpointCallback(func(handler CheckpointCommitHandler) {
			saver = handler
		}))

	for i := int64(0); i < trcRetry; i++ {
		if c.IsCtxDone() {
			return utils.Errorf("context is done, cannot continue transaction")
		}
		rsp, err := callAi(
			NewAIRequest(
				c.RetryPromptBuilder(prompt, postHandlerErr),
				append(requestOpts, WithAIRequest_SeqId(seq))...,
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
		if c.IsCtxDone() {
			return utils.Errorf("context is done, cannot continue transaction")
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
	requestOpts ...AIRequestOption,
) error {
	return CallAITransaction(c, prompt, callAi, postHandler, requestOpts...)
}

var retryPromptTemplate = `
{{ .RawPrompt }}

# 错误处理：
注意，你生成的结果在之前已经犯过错误，这是上次失败的原因：
{{ .RetryReason }}
请你在生成结果时，注意不要再犯同样的错误。
# 如何修正？
如果要生成 action/@action JSON 可以参考后面的案例，注意格式遵守：{"@action": "...", ... }
`

func (c *Config) RetryPromptBuilder(rawPrompt string, retryErr error) string {
	if retryErr == nil {
		return rawPrompt
	}
	templateData := map[string]interface{}{
		"RetryReason": retryErr.Error(),
		"RawPrompt":   rawPrompt,
	}
	res, err := c.quickBuildPrompt(retryPromptTemplate, templateData)
	if err != nil {
		c.EmitError("failed to build retry prompt: %v", err)
		return rawPrompt
	}
	return res
}
