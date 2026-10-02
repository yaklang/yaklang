package loop_coordinator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// PlanNode is the Yakit wire contract, not an executable legacy AiTask.
// Only the controller's immutable briefs can be dispatched to workers.
type PlanNode struct {
	TaskID               string      `json:"task_id,omitempty"`
	Index                string      `json:"index"`
	Name                 string      `json:"name"`
	Goal                 string      `json:"goal"`
	Identifier           string      `json:"semantic_identifier"`
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
				MainName       string            `json:"main_task"`
				MainGoal       string            `json:"main_task_goal"`
				MainIdentifier string            `json:"main_task_identifier"`
				SubName        string            `json:"subtask_name"`
				SubGoal        string            `json:"subtask_goal"`
				SubIdentifier  string            `json:"subtask_identifier"`
				Tasks          []json.RawMessage `json:"tasks"`
				Children       []json.RawMessage `json:"sub_subtasks"`
				DependsOn      []string          `json:"depends_on"`
			}
			if err := json.Unmarshal(raw, &node); err != nil {
				return nil, err
			}
			n := &PlanNode{Name: node.Name, Goal: node.Goal, Identifier: node.Identifier, DependsOn: node.DependsOn}
			if n.Name == "" {
				if top {
					n.Name, n.Goal, n.Identifier = node.MainName, node.MainGoal, node.MainIdentifier
				} else {
					n.Name, n.Goal, n.Identifier = node.SubName, node.SubGoal, node.SubIdentifier
				}
			}
			children := node.Children
			if top {
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
		if n.Identifier == "" {
			n.Identifier = aicommon.SanitizeTaskName(n.Name)
		}
		if n.Identifier == "" || identifiers[n.Identifier] {
			return fmt.Errorf("duplicate or empty semantic identifier %q", n.Identifier)
		}
		identifiers[n.Identifier] = true
		if id := oldIDs[n.Identifier]; id != "" {
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
	// A dependency on a group means all leaves in that group. Group-level
	// prerequisites apply to every descendant, matching the UI's nested tree.
	var build func(*PlanNode, []string) error
	build = func(n *PlanNode, inherited []string) error {
		deps := append(append([]string{}, inherited...), n.DependsOn...)
		if len(n.Subtasks) > 0 {
			for _, c := range n.Subtasks {
				if err := build(c, deps); err != nil {
					return err
				}
			}
			return nil
		}
		brief := Task{ID: n.TaskID, Index: n.Index, Name: n.Name, Goal: n.Goal}
		seen := map[string]bool{}
		for _, ref := range deps {
			ref = strings.TrimSpace(ref)
			target := refs[ref]
			if target == nil || ambiguous[ref] {
				return fmt.Errorf("unknown or ambiguous dependency %q", ref)
			}
			target.walk(func(t *PlanNode) {
				if len(t.Subtasks) == 0 && !seen[t.TaskID] {
					brief.DependsOn = append(brief.DependsOn, t.TaskID)
					seen[t.TaskID] = true
				}
			})
		}
		p.Tasks = append(p.Tasks, brief)
		return nil
	}
	if err := build(root, nil); err != nil {
		return nil, err
	}
	if err := validate(p); err != nil {
		return nil, err
	}
	var err error
	p.Tree, err = json.Marshal(root)
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
		a.ID, a.State, a.Seen, a.Result = 0, Pending, false, Result{}
		a.ReviewReason = "User requested recovery from this task."
		s.Attempts[key] = a
	}
	s.Finished = false
	return nil
}
