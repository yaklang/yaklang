package coordinator

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/schema"
)

// subForgeInteractiveEvent exposes only interaction endpoints. Ordinary child
// execution events stay private; the parent ToolCaller owns progress and result.
func subForgeInteractiveEvent(event *schema.AiOutputEvent) *schema.AiOutputEvent {
	switch event.Type {
	case schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, schema.EVENT_TYPE_TASK_REVIEW_REQUIRE,
		schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE,
		schema.EVENT_TYPE_PERMISSION_REQUIRE:
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
			"prompt":  fmt.Sprintf("Sub-Forge review (%s):\n```json\n%s\n```", event.Type, event.Content),
			"options": options,
		})
		forwarded := *event
		forwarded.Type = schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE
		forwarded.NodeId = "require-user-interact"
		forwarded.Content = content
		return &forwarded
	case schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE, schema.EVENT_TYPE_REVIEW_RELEASE:
		return event
	default:
		return nil
	}
}
