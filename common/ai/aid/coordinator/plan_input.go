package coordinator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonextractor"
)

// ExtractPlan keeps the Yak argument shape. Parsing never creates a runtime or
// asks a model to regenerate a supplied plan.
func ExtractPlan(_ any, text string) (*PlanResponse, error) {
	action, err := aicommon.ExtractAction(text, "plan")
	var data []byte
	if err == nil {
		data, err = json.Marshal(action.GetParams())
		if err != nil {
			return nil, err
		}
	} else {
		// Static presets also arrive as a bare or fenced plan definition. This
		// is local parsing, not an AI request to infer or rewrite the plan.
		for _, raw := range jsonextractor.ExtractStandardJSON(text) {
			var candidate map[string]any
			if json.Unmarshal([]byte(raw), &candidate) != nil {
				continue
			}
			if marker, exists := candidate["@action"]; exists && marker != "plan" {
				continue
			}
			if candidate["tasks"] != nil || candidate["subtasks"] != nil || candidate["root_task"] != nil {
				data = []byte(raw)
				break
			}
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("extract preset plan: %w", err)
		}
	}
	p, err := ParsePlan(string(data), "", nil)
	if err != nil {
		return nil, fmt.Errorf("validate preset plan: %w", err)
	}
	var root PlanNode
	if err := json.Unmarshal(p.Tree, &root); err != nil {
		return nil, err
	}
	return &PlanResponse{RootTask: &root, Document: PlanDocument(&root)}, nil
}

// PlanDocument is a deterministic compatibility document for static task trees.
func PlanDocument(root *PlanNode) string {
	if root == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n", root.Name, root.Goal)
	var visit func(*PlanNode, int)
	visit = func(n *PlanNode, depth int) {
		fmt.Fprintf(&b, "\n%s- %s：%s\n", strings.Repeat("  ", depth), n.Name, n.Goal)
		if len(n.DependsOn) > 0 {
			fmt.Fprintf(&b, "%s  前置任务：%s\n", strings.Repeat("  ", depth), strings.Join(n.DependsOn, ", "))
		}
		for _, child := range n.Subtasks {
			visit(child, depth+1)
		}
	}
	for _, child := range root.Subtasks {
		visit(child, 0)
	}
	return b.String()
}
