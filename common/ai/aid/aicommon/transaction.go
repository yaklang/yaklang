package aicommon

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func CallAITransaction(
	c AICallerConfigIf,
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
	requestOpts ...AIRequestOption,
) error {
	var seq int64
	var saver CheckpointCommitHandler
	var trcRetry int64 = 3
	if c != nil {
		trcRetry = c.GetAITransactionAutoRetryCount()
	}
	if trcRetry <= 0 {
		trcRetry = 3
	}
	var postHandlerErr error

	emitter := c.GetEmitter()

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
		finalPrompt := c.RetryPromptBuilder(prompt, postHandlerErr)
		// 调试打印：输出完整的 prompt（仅在测试环境或调试模式下）
		if i == 0 {
			emitter.EmitInfo("[DEBUG] AI Transaction Prompt (seq=%d, attempt=%d):\n%s", seq, i+1, finalPrompt)
		} else {
			emitter.EmitInfo("[DEBUG] AI Transaction Prompt Retry (seq=%d, attempt=%d):\n%s", seq, i+1, utils.ShrinkString(finalPrompt, 512))
		}
		rsp, err := callAi(
			NewAIRequest(
				finalPrompt,
				append(requestOpts, WithAIRequest_SeqId(seq))...,
			))
		if err != nil {
			emitter.EmitError("call ai api error: %v, retry and block it", err)
			select {
			case <-c.GetContext().Done():
				return err
			case <-time.After(100 * time.Millisecond):
				emitter.EmitWarning("call ai transaction retry")
				continue
			}
		}
		if c.IsCtxDone() {
			return utils.Errorf("context is done, cannot continue transaction")
		}
		postHandlerErr = postHandler(rsp)
		if postHandlerErr != nil {
			emitter.EmitError("ai transaction in postHandler error: %v, retry and block it, prompts: %v", postHandlerErr, utils.ShrinkString(finalPrompt, 512))
			select {
			case <-c.GetContext().Done():
				return postHandlerErr
			case <-time.After(100 * time.Millisecond):
				emitter.EmitWarning("call ai transaction retry")
				continue
			}
		}
		if saver != nil {
			cp, err := saver()
			if cp == nil {
				emitter.EmitError("cannot save checkpoint")
				return err
			} else {
				emitter.EmitInfo("checkpoint cached in database: %v:%v", utils.ShrinkString(cp.CoordinatorUuid, 12), cp.Seq)
			}
		}
		return nil
	}
	return utils.Errorf("max retry count[%v] reached in transaction", trcRetry)
}
