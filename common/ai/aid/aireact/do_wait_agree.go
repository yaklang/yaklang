package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/schema"
	"time"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

const SKIP_AI_REVIEW = "skip_ai_review"

func (r *ReActConfig) DoWaitAgree(ctx context.Context, endpoint *aicommon.Endpoint) {
	skipAIReview := utils.GetContextKeyBool(ctx, SKIP_AI_REVIEW)

	if r.reviewPolicy == "" {
		r.reviewPolicy = aicommon.AgreePolicyManual
	}

	interactiveId := endpoint.GetId()

	switch r.reviewPolicy {
	case aicommon.AgreePolicyYOLO, aicommon.AgreePolicyAuto:
		r.Emitter.EmitInfo("yolo policy auto agree all")
		log.Infof("Auto-approving tool usage (non-interactive mode)")
		// Set default continue response
		endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
		endpoint.Release()
		return
	case aicommon.AgreePolicyAI, aicommon.AgreePolicyAIAuto:
		if skipAIReview {
			// Default behavior: wait for user interaction
			endpoint.Wait()
			return
		}
		go func() {
			go func() {
				r.Emitter.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_START, "ai-reviewer", map[string]any{
					"interactive_id": interactiveId,
				})
				endOnce := utils.NewOnce()
				endNormally := func(score float64, level string, reason string) {
					endOnce.Do(func() {
						r.Emitter.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_END, "ai-reviewer", map[string]any{
							"score":          score,
							"reason":         reason,
							"interactive_id": interactiveId,
							"level":          level,
						})
					})
				}
				defer func() {
					endNormally(1.0, "high", "review interrupted")
				}()

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
					stream := rsp.GetOutputStreamReader("review", false, r.Emitter)
					// stream = io.TeeReader(stream, os.Stdout)
					action, err := aicommon.ExtractActionFromStream(stream, "review_tool_call", "object")
					if err != nil {
						return utils.Errorf("error extracting action from stream: %v", err)
					}
					score = action.GetFloat("risk_score")
					reason = action.GetString("reason")
					if reason == "" {
						return utils.Error("invalid review score or reason: empty reason")
					}
					if score < 0 {
						score = 0.0
					}
					if score > 1 {
						score = 1.0
					}
					log.Infof("Auto-review evaluating: score=%.2f, reason=%s", score, reason)
					return nil
				})
				if err != nil {
					endNormally(1, "high", "review failed: "+err.Error())
					log.Errorf("error during auto-review: %v", err)
					return
				}
				if score <= 0.4 {
					var duSec time.Duration = 1
					r.Emitter.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_COUNTDOWN, "ai-reviewer", map[string]any{
						"seconds":        int(duSec),
						"interactive_id": interactiveId,
						"score":          score,
						"level":          "low",
					})
					r.Emitter.EmitInfo("Auto-review score is low, suggesting to continue in " + fmt.Sprint(int(duSec)) + " seconds...")
					endNormally(score, "low", reason)
					time.Sleep(duSec * time.Second) // Simulate a delay for user to read the message
					endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
					endpoint.Release()
				} else if score > 0.4 && score <= 0.7 {
					var duSec time.Duration = 6
					r.Emitter.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_COUNTDOWN, "ai-reviewer", map[string]any{
						"seconds":        int(duSec),
						"interactive_id": interactiveId,
						"score":          score,
						"level":          "middle",
					})
					endNormally(score, "middle", reason)
					r.Emitter.EmitInfo("Auto-review score is middle, suggesting to continue in " + fmt.Sprint(int(duSec)) + " seconds...")
					time.Sleep(duSec * time.Second) // Simulate a delay for user to read the message
					endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
					endpoint.Release()
				} else {
					r.Emitter.EmitInfo("Auto-review score is high, suggesting to handled by user")
					endNormally(score, "high", reason)
				}
			}()
		}()
		endpoint.Wait()
	case aicommon.AgreePolicyManual:
		fallthrough
	default:
		endpoint.Wait()
	}
}
