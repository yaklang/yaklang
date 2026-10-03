package coordinator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// PlanNode is the Yakit wire contract, not an executable legacy AiTask.
// Only the controller's immutable briefs can be dispatched to workers.
type PlanNode struct {
	TaskID     string `json:"task_id,omitempty"`
	Index      string `json:"index"`
	Name       string `json:"name"`
	Goal       string `json:"goal"`
	Identifier string `json:"semantic_identifier"`
	// Yakit initializes these review fields on mount. Omitting them makes an
	// untouched plan look edited and changes the approval payload shape.
	Description          string      `json:"description"`
	Tools                []string    `json:"tools"`
	DependsOn            []string    `json:"depends_on,omitempty"`
	Subtasks             []*PlanNode `json:"subtasks,omitempty"`
	Progress             string      `json:"progress"`
	Summary              string      `json:"summary"`
	TaskSummary          string      `json:"task_summary,omitempty"`
	ShortSummary         string      `json:"short_summary,omitempty"`
	LongSummary          string      `json:"long_summary,omitempty"`
	TotalToolCallCount   int64       `json:"total_tool_call_count"`
	SuccessToolCallCount int         `json:"success_tool_call_count"`
	FailToolCallCount    int         `json:"fail_tool_call_count"`
}

func (n *PlanNode) walk(f func(*PlanNode)) {
	f(n)
	for _, c := range n.Subtasks {
		c.walk(f)
	}
}

// Only the frontend adapter adds editor defaults; internal trees are stateless.
func displayPlanTree(p *Plan) (json.RawMessage, error) {
	var root PlanNode
	if err := json.Unmarshal(p.Tree, &root); err != nil {
		return nil, err
	}
	root.walk(func(n *PlanNode) {
		if n.Tools == nil {
			n.Tools = []string{}
		}
	})
	return json.Marshal(root)
}

// ParseReviewedPlan applies Yakit's soft deletions before validating the
// approved tree. UI-only removal flags never enter stored or executable plans.
// Dependencies on deleted tasks remain invalid and must be corrected explicitly.
func ParseReviewedPlan(data, document string, previous *Plan) (*Plan, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(data), &root); err != nil || len(root) == 0 {
		return nil, fmt.Errorf("reviewed plan must be a nonempty object")
	}
	if raw, exists := root["root_task"]; exists {
		var ok bool
		root, ok = raw.(map[string]any)
		if !ok || len(root) == 0 {
			return nil, fmt.Errorf("root_task must be a nonempty object")
		}
	}
	var prune func(map[string]any) (bool, error)
	prune = func(node map[string]any) (bool, error) {
		if removed, _ := node["isRemove"].(bool); removed {
			return false, nil
		}
		delete(node, "isRemove")
		if raw := node["subtasks"]; raw != nil {
			children, ok := raw.([]any)
			if !ok {
				return false, fmt.Errorf("subtasks must be an array")
			}
			kept := make([]any, 0, len(children))
			for _, child := range children {
				object, ok := child.(map[string]any)
				if !ok || len(object) == 0 {
					return false, fmt.Errorf("plan task must be a nonempty object")
				}
				keep, err := prune(object)
				if err != nil {
					return false, err
				}
				if keep {
					kept = append(kept, object)
				}
			}
			if len(children) > 0 && len(kept) == 0 {
				return false, nil // An emptied group must not become executable.
			}
			node["subtasks"] = kept
		}
		return true, nil
	}
	keep, err := prune(root)
	if err != nil {
		return nil, err
	}
	if !keep {
		return nil, fmt.Errorf("reviewed plan requires executable tasks")
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return ParsePlan(string(raw), document, previous)
}

// ParsePlan accepts native plan arguments and the existing edited root_task
// payload. Historical plan-data JSON is decoded only at this wire boundary.
func ParsePlan(data, document string, previous *Plan) (*Plan, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("plan must be a JSON object")
	}
	if raw := fields["root_task"]; raw != nil {
		return ParsePlan(string(raw), document, previous)
	}
	var root *PlanNode
	if fields["subtasks"] != nil || fields["task_id"] != nil {
		if err := json.Unmarshal([]byte(data), &root); err != nil {
			return nil, err
		}
	} else {
		var parse func(json.RawMessage, bool) (*PlanNode, error)
		parse = func(raw json.RawMessage, top bool) (*PlanNode, error) {
			var node struct {
				Name           string            `json:"name"`
				Goal           string            `json:"goal"`
				Identifier     string            `json:"identifier"`
				SemanticID     string            `json:"semantic_identifier"`
				TaskID         string            `json:"task_id"`
				MainName       string            `json:"main_task"`
				MainGoal       string            `json:"main_task_goal"`
				MainIdentifier string            `json:"main_task_identifier"`
				SubName        string            `json:"subtask_name"`
				SubGoal        string            `json:"subtask_goal"`
				SubIdentifier  string            `json:"subtask_identifier"`
				Tasks          []json.RawMessage `json:"tasks"`
				Children       []json.RawMessage `json:"sub_subtasks"`
				Subtasks       []json.RawMessage `json:"subtasks"`
				DependsOn      []string          `json:"depends_on"`
			}
			if err := json.Unmarshal(raw, &node); err != nil {
				return nil, err
			}
			n := &PlanNode{TaskID: node.TaskID, Name: node.Name, Goal: node.Goal, Identifier: node.Identifier, DependsOn: node.DependsOn}
			if n.Identifier == "" {
				n.Identifier = node.SemanticID
			}
			if n.Name == "" {
				if top {
					n.Name, n.Goal, n.Identifier = node.MainName, node.MainGoal, node.MainIdentifier
				} else {
					n.Name, n.Goal, n.Identifier = node.SubName, node.SubGoal, node.SubIdentifier
				}
			}
			children := node.Children
			if children == nil {
				children = node.Subtasks
			}
			if top || children == nil {
				children = node.Tasks
			}
			for _, raw := range children {
				child, err := parse(raw, false)
				if err != nil {
					return nil, err
				}
				n.Subtasks = append(n.Subtasks, child)
			}
			return n, nil
		}
		var err error
		root, err = parse(json.RawMessage(data), true)
		if err != nil {
			return nil, err
		}
	}
	if root == nil || len(root.Subtasks) == 0 {
		return nil, fmt.Errorf("plan requires executable tasks")
	}
	oldIDs := map[string]string{}
	if previous != nil {
		var old *PlanNode
		if err := json.Unmarshal(previous.Tree, &old); err != nil || old == nil {
			return nil, fmt.Errorf("invalid previous plan tree")
		}
		old.walk(func(n *PlanNode) { oldIDs[n.Identifier] = n.TaskID })
	}
	refs := map[string]*PlanNode{}
	ambiguous := map[string]bool{}
	ids, identifiers := map[string]bool{}, map[string]bool{}
	var normalize func(*PlanNode, string) error
	normalize = func(n *PlanNode, index string) error {
		if n == nil || strings.TrimSpace(n.Name) == "" || strings.TrimSpace(n.Goal) == "" {
			return fmt.Errorf("every plan node requires name and goal")
		}
		n.Index = index
		if n.Tools == nil {
			n.Tools = []string{}
		}
		if n.Identifier == "" {
			n.Identifier = aicommon.SanitizeTaskName(n.Name)
		}
		if n.Identifier == "" || identifiers[n.Identifier] {
			return fmt.Errorf("duplicate or empty semantic identifier %q", n.Identifier)
		}
		identifiers[n.Identifier] = true
		if id := oldIDs[n.Identifier]; n.TaskID == "" && id != "" {
			n.TaskID = id
		}
		if n.TaskID == "" {
			n.TaskID = "plan-task" + uuid.NewString()
		}
		if ids[n.TaskID] {
			return fmt.Errorf("duplicate task id %q", n.TaskID)
		}
		ids[n.TaskID] = true
		for _, ref := range []string{n.TaskID, n.Index, n.Identifier, n.Name} {
			if existing := refs[ref]; existing != nil && existing != n {
				ambiguous[ref] = true
			}
			refs[ref] = n
		}
		for i, child := range n.Subtasks {
			next := fmt.Sprint(i + 1)
			if index != "0" {
				next = index + "-" + next
			}
			if err := normalize(child, next); err != nil {
				return err
			}
		}
		return nil
	}
	if err := normalize(root, "0"); err != nil {
		return nil, err
	}
	p := &Plan{Document: document}
	// Preserve the old DAG semantics: group references expand to all leaves,
	// while a group's prerequisites attach only to its internal entry leaves.
	leaves := map[*PlanNode][]*PlanNode{}
	var collect func(*PlanNode) []*PlanNode
	collect = func(n *PlanNode) []*PlanNode {
		if len(n.Subtasks) == 0 {
			leaves[n] = []*PlanNode{n}
		} else {
			for _, child := range n.Subtasks {
				leaves[n] = append(leaves[n], collect(child)...)
			}
		}
		return leaves[n]
	}
	collect(root)
	briefs := map[string]*Task{}
	resolve := func(deps []string) ([]string, error) {
		var ids []string
		seen := map[string]bool{}
		for _, ref := range deps {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			target := refs[ref]
			if target == nil || ambiguous[ref] {
				return nil, fmt.Errorf("unknown or ambiguous dependency %q", ref)
			}
			for _, leaf := range leaves[target] {
				if !seen[leaf.TaskID] {
					ids = append(ids, leaf.TaskID)
					seen[leaf.TaskID] = true
				}
			}
		}
		return ids, nil
	}
	var build func(*PlanNode) error
	build = func(n *PlanNode) error {
		if len(n.Subtasks) > 0 {
			for _, c := range n.Subtasks {
				if err := build(c); err != nil {
					return err
				}
			}
		} else {
			briefs[n.TaskID] = &Task{ID: n.TaskID, Index: n.Index, Name: n.Name, Goal: n.Goal}
		}
		deps, err := resolve(n.DependsOn)
		if err != nil {
			return err
		}
		subtree := map[string]bool{}
		for _, leaf := range leaves[n] {
			subtree[leaf.TaskID] = true
		}
		for _, leaf := range leaves[n] {
			brief := briefs[leaf.TaskID]
			entry := true
			for _, dep := range brief.DependsOn {
				if subtree[dep] {
					entry = false
					break
				}
			}
			if entry {
				brief.DependsOn = append(brief.DependsOn, deps...)
			}
		}
		return nil
	}
	if err := build(root); err != nil {
		return nil, err
	}
	order := map[string]int{}
	for i, leaf := range leaves[root] {
		order[leaf.TaskID] = i
	}
	for _, leaf := range leaves[root] {
		brief := *briefs[leaf.TaskID]
		sort.Slice(brief.DependsOn, func(i, j int) bool { return order[brief.DependsOn[i]] < order[brief.DependsOn[j]] })
		var unique []string
		for _, dep := range brief.DependsOn {
			if len(unique) == 0 || dep != unique[len(unique)-1] {
				unique = append(unique, dep)
			}
		}
		brief.DependsOn = unique
		p.Tasks = append(p.Tasks, brief)
	}
	if err := validate(p); err != nil {
		return nil, err
	}
	var err error
	p.Tree, err = json.Marshal(definitionTree(root))
	return p, err
}

func resetCoordinatorRecovery(s *Snapshot, reference string) error {
	if reference == "" {
		return nil
	}
	id := ""
	for key, a := range s.Attempts {
		if key == reference || a.Task.Index == reference {
			id = key
			break
		}
	}
	if id == "" {
		return fmt.Errorf("recovery start task %q not found", reference)
	}
	affected := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for key, a := range s.Attempts {
			for _, dep := range a.Task.DependsOn {
				if affected[dep] && !affected[key] {
					affected[key] = true
					changed = true
				}
			}
		}
	}
	for key := range affected {
		a := s.Attempts[key]
		a.ID, a.State, a.Result = 0, Pending, Result{}
		a.ReviewReason = "User requested recovery from this task."
		s.Attempts[key] = a
	}
	s.Finished = false
	return nil
}
