package aicommon

import (
	"fmt"
	"strings"
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
	var lastErr error
	var lastRsp *AIResponse

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

		utils.Debug(func() {
			if i == 0 {
				emitter.EmitInfo("[DEBUG] AI Transaction Prompt (seq=%d, attempt=%d):\n%s", seq, i+1, finalPrompt)
			} else {
				emitter.EmitInfo("[DEBUG] AI Transaction Prompt Retry (seq=%d, attempt=%d):\n%s", seq, i+1, utils.ShrinkString(finalPrompt, 512))
			}
		})

		req := NewAIRequest(
			finalPrompt,
			append(requestOpts, WithAIRequest_SeqId(seq))...,
		)
		if postHandlerErr != nil && req != nil {
			promptFallback := req.GetPromptFallback()
			if promptFallback != nil {
				req.SetPromptFallback(func(expectedContextSize int, currentContextSize int) (string, error) {
					prompt, err := promptFallback(expectedContextSize, currentContextSize)
					if err != nil || strings.TrimSpace(prompt) == "" {
						return prompt, err
					}
					return c.RetryPromptBuilder(prompt, postHandlerErr), nil
				})
			}
		}

		rsp, err := callAi(req)
		if err != nil {
			lastErr = err
			lastRsp = rsp
			emitter.EmitError("call ai api error (attempt %d/%d): %v", i+1, trcRetry, err)
			select {
			case <-c.GetContext().Done():
				return err
			case <-time.After(100 * time.Millisecond):
				emitter.EmitWarning("call ai transaction retry (attempt %d/%d)", i+1, trcRetry)
				continue
			}
		}
		if c.IsCtxDone() {
			return utils.Errorf("context is done, cannot continue transaction")
		}
		lastRsp = rsp
		postHandlerErr = postHandler(rsp)
		if postHandlerErr != nil {
			lastErr = postHandlerErr
			emitter.EmitError("ai transaction postHandler error (attempt %d/%d): %v", i+1, trcRetry, postHandlerErr)
			select {
			case <-c.GetContext().Done():
				return postHandlerErr
			case <-time.After(100 * time.Millisecond):
				emitter.EmitWarning("call ai transaction retry (attempt %d/%d)", i+1, trcRetry)
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

	var modelInfo string
	if lastRsp != nil {
		provider := lastRsp.GetProviderName()
		model := lastRsp.GetModelName()
		if provider != "" || model != "" {
			modelInfo = fmt.Sprintf(" (model: %s:%s)", provider, model)
		}
	}
	finalErrMsg := fmt.Sprintf(
		"[AI Transaction Failed] After %d attempts%s, the AI interaction could not complete.\n"+
			"Last error: %v\n\n"+
			"Suggested actions:\n"+
			"1. Check if the current AI model is working properly\n"+
			"2. Try switching to a different AI model\n"+
			"3. Simplify the task or reduce the prompt complexity\n"+
			"4. Check network connectivity and API rate limits",
		trcRetry, modelInfo, lastErr,
	)
	if lastRsp != nil {
		rawDump := lastRsp.GetRawHTTPResponseDump()
		if rawDump != "" {
			finalErrMsg += "\n\n--- Last Raw HTTP Response ---\n" + utils.ShrinkString(rawDump, 4096)
		}
	}
	emitter.EmitDefaultStreamEvent("ai-error", strings.NewReader(finalErrMsg), "")

	if lastErr != nil {
		return utils.Errorf("max retry count[%v] reached in transaction, last error: %v", trcRetry, lastErr)
	}
	return utils.Errorf("max retry count[%v] reached in transaction", trcRetry)
}
