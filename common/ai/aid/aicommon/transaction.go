package aicommon

import (
	"github.com/yaklang/yaklang/common/utils"
	"time"
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
		rsp, err := callAi(
			NewAIRequest(
				c.RetryPromptBuilder(prompt, postHandlerErr),
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
			emitter.EmitError("ai transaction in postHandler error: %v, retry and block it", postHandlerErr)
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
