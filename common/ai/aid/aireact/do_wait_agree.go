package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

func (r *ReActConfig) DoWaitAgree(ctx context.Context, endpoint *aicommon.Endpoint) {
	if r.autoAIReview {
		go func() {
			// In auto-review mode, automatically approve the request
			materials := endpoint.GetReviewMaterials()
			params := materials.GetObject("params")
			paramRaw, _ := json.MarshalIndent(params, "", "  ")
			prompt, err := r.promptManager.GenerateAIReviewPrompt(
				"ai request tool-call, start to review",
				fmt.Sprintf(
					`[%v]:%v`,
					materials.GetString("tool"),
					materials.GetString("tool_description"),
				),
				string(paramRaw),
			)
			if err != nil {
				log.Errorf("error generating AI review prompt: %v", err)
				return
			}
			var score float64
			var reason string
			err = aicommon.CallAITransaction(r, prompt, r.CallAI, func(rsp *aicommon.AIResponse) error {
				stream := rsp.GetOutputStreamReader("review", true, r.Emitter)
				// stream = io.TeeReader(stream, os.Stdout)
				action, err := aicommon.ExtractActionFromStream(stream, "review_tool_call", "object")
				if err != nil {
					return utils.Errorf("error extracting action from stream: %v", err)
				}
				score = action.GetFloat("risk_score")
				reason = action.GetString("reason")
				if score <= 0 || reason == "" {
					return utils.Errorf("invalid review score or reason: score=%f, reason=%s", score, reason)
				}
				log.Infof("Auto-review evaluating: score=%.2f, reason=%s", score, reason)
				return nil
			})
			if err != nil {
				log.Errorf("error during auto-review: %v", err)
				return
			}
			if score <= 0.4 {
				r.Emitter.EmitInfo("Auto-review score is low, suggesting to continue in 3 seconds...")
				time.Sleep(3 * time.Second) // Simulate a delay for user to read the message
				endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
				endpoint.Release()
			} else if score >= 0.7 {
				r.Emitter.EmitInfo("Auto-review score is high, suggesting to proceed in 6 seconds...")
				time.Sleep(6 * time.Second) // Simulate a delay for user to read the message
				endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
				endpoint.Release()
			} else {
				r.Emitter.EmitInfo("Auto-review score is high, suggesting to handled by user")
			}
		}()
	}

	// In auto-approve mode, automatically approve the request
	if r.autoApproveTools {
		log.Infof("Auto-approving tool usage (non-interactive mode)")
		// Set default continue response
		endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
		endpoint.Release()
		return
	}

	// Default behavior: wait for user interaction
	endpoint.Wait()
}
