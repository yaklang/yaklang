package coordinator

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// subForgeInteractiveEvent exposes only interaction endpoints. Ordinary child
// execution events stay private; the parent ToolCaller owns progress and result.
func subForgeInteractiveEvent(event *schema.AiOutputEvent) *schema.AiOutputEvent {
	switch event.Type {
	case schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE:
		// Legacy plan-review panels replace the current task tree on approval.
		// Use their existing generic question card instead, retaining the endpoint,
		// review materials and exact selector values used by the child reviewer.
		var materials struct {
			ID        string `json:"id"`
			Selectors []struct {
				Value string `json:"value"`
			} `json:"selectors"`
		}
		if err := json.Unmarshal(event.Content, &materials); err != nil {
			return nil
		}
		options := make([]map[string]any, 0, len(materials.Selectors))
		for i, selector := range materials.Selectors {
			options = append(options, map[string]any{"index": i + 1, "prompt_title": selector.Value})
		}
		content, _ := json.Marshal(map[string]any{
			"id":      materials.ID,
			"prompt":  fmt.Sprintf("Sub-Forge review (%s). Select an option and submit; append feedback after the colon. To edit the plan, replace the entire input with a JSON object containing suggestion, reviewed-task-tree and optional plans.document.\n```json\n%s\n```", event.Type, event.Content),
			"options": options,
		})
		forwarded := *event
		forwarded.Type = schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE
		forwarded.NodeId = "require-user-interact"
		forwarded.Content = content
		return &forwarded
	case schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE, schema.EVENT_TYPE_REVIEW_RELEASE,
		schema.EVENT_TYPE_TASK_REVIEW_REQUIRE, schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE:
		return event
	default:
		return nil
	}
}

// subForgePlanReviewInput translates the existing generic card's "value: text"
// submission without changing the child's review endpoint or approving for it.
// Full JSON submissions retain structured plan edits supported by the endpoint.
func subForgePlanReviewInput(event *ypb.AIInputEvent, review *schema.AiOutputEvent) (*ypb.AIInputEvent, bool) {
	var params map[string]any
	if json.Unmarshal([]byte(event.InteractiveJSONInput), &params) != nil {
		return nil, false
	}
	text, _ := params["suggestion"].(string)
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "{") {
		if json.Unmarshal([]byte(text), &params) != nil {
			return nil, false
		}
	} else if value, feedback, found := strings.Cut(text, ":"); found {
		params["suggestion"] = strings.TrimSpace(value)
		if strings.TrimSpace(feedback) != "" {
			params["extra_prompt"] = strings.TrimSpace(feedback)
		}
	}
	var materials struct {
		Selectors []struct {
			Value string `json:"value"`
		} `json:"selectors"`
	}
	if json.Unmarshal(review.Content, &materials) != nil {
		return nil, false
	}
	suggestion, _ := params["suggestion"].(string)
	allowed := false
	for _, selector := range materials.Selectors {
		if suggestion == selector.Value {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, false
	}
	if suggestion == "freedom-review" {
		tree, _ := params["reviewed-task-tree"].(map[string]any)
		if len(tree) == 0 {
			return nil, false
		}
	}
	content, err := json.Marshal(params)
	if err != nil {
		return nil, false
	}
	forwarded := proto.Clone(event).(*ypb.AIInputEvent)
	forwarded.InteractiveJSONInput = string(content)
	return forwarded, true
}
