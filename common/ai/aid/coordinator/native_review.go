package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// NativeRiskReview preserves the existing AI/manual/YOLO policy and UI review
// events. AI risk judging is one native call, not another ReAct loop or JSON
// response-format transaction. Ordinary response text never authorizes a tool.
func NativeRiskReview(ctx context.Context, cfg *aicommon.Config, ep *aicommon.Endpoint) (*aicommon.Action, error) {
	materials, _ := json.Marshal(ep.GetReviewMaterials())
	tool := aispec.Tool{Type: "function", Function: aispec.ToolFunction{Name: "review_risk", Description: "Judge the requested operation using actual review materials. risk_score is 0 (low) to 1 (high).", Parameters: map[string]any{
		"type": "object", "properties": map[string]any{"risk_score": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "reason": map[string]any{"type": "string"}}, "required": []string{"risk_score", "reason"}, "additionalProperties": false,
	}}}
	var mu sync.Mutex
	var name, id, arguments, finish string
	var index int
	var seen bool
	var collectErr error
	var action *aicommon.Action
	reset := func() {
		mu.Lock()
		name, id, arguments, finish = "", "", "", ""
		seen, collectErr = false, nil
		mu.Unlock()
	}
	prompt := "Review the operation below. Respect the user's authorization and the supplied scope; assess concrete side effects. Call review_risk once with a score and reason.\n" + string(materials)
	err := aicommon.CallAITransaction(cfg, prompt, func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		reset()
		aicommon.WithAIRequest_Context(ctx)(req)
		aicommon.WithAIRequest_CallerLabel("coordinator:risk-review")(req)
		aicommon.WithAIRequest_ExtraSpecOpts(aispec.WithContext(ctx), aispec.WithTools([]aispec.Tool{tool}), aispec.WithToolChoice("required"), aispec.AIConfigOption(func(c *aispec.AIConfig) {
			previous := c.RawHTTPResponseHeaderCallback
			c.RawHTTPResponseHeaderCallback = func(header []byte) {
				if previous != nil {
					previous(header)
				}
				reset()
			}
		}), aispec.WithToolCallCallback(func(calls []*aispec.ToolCall) {
			mu.Lock()
			defer mu.Unlock()
			for _, call := range calls {
				if call == nil {
					continue
				}
				if seen && (call.Index != index || (id != "" && call.ID != "" && call.ID != id)) {
					collectErr = fmt.Errorf("risk review returned multiple calls")
					continue
				}
				seen = true
				index = call.Index
				if call.ID != "" {
					id = call.ID
				}
				if part := call.Function.Name; part != "" {
					if part == "review_risk" && strings.HasPrefix(part, name) {
						name = part
					} else {
						name += part
					}
				}
				arguments += call.Function.Arguments
			}
		}), aispec.WithFinishReasonCallback(func(reason string, _ []byte) { mu.Lock(); finish = reason; mu.Unlock() }))(req)
		return cfg.CallQualityPriorityAI(req)
	}, func(resp *aicommon.AIResponse) error {
		if _, err := io.Copy(io.Discard, resp.GetUnboundStreamReader(false)); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		if collectErr != nil {
			return collectErr
		}
		if !seen || name != "review_risk" || finish != "tool_calls" {
			return fmt.Errorf("risk review requires a native review_risk function call")
		}
		var result struct {
			Score  *float64 `json:"risk_score"`
			Reason string   `json:"reason"`
		}
		if err := json.Unmarshal([]byte(arguments), &result); err != nil {
			return err
		}
		if result.Score == nil || math.IsNaN(*result.Score) || *result.Score < 0 || *result.Score > 1 || strings.TrimSpace(result.Reason) == "" {
			return fmt.Errorf("invalid native risk review result")
		}
		var err error
		action, err = aicommon.ExtractAction(`{"@action":"review_risk","risk_score":`+fmt.Sprint(*result.Score)+`,"reason":`+jsonString(result.Reason)+`}`, "review_risk")
		return err
	})
	return action, err
}

func jsonString(value string) string { data, _ := json.Marshal(value); return string(data) }
