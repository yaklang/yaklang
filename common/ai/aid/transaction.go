package aid

import (
	"github.com/yaklang/yaklang/common/utils"
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
	for i := int64(0); i < trcRetry; i++ {
		rsp, err := callAi(
			NewAIRequest(
				prompt,
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
		err = postHandler(rsp)
		if err != nil {
			c.EmitError("ai transaction in postHandler error: %v, retry and block it", err)
			select {
			case <-c.ctx.Done():
				return err
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
