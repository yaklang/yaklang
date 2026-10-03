package coordinator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// planDefinitionNode exposes task briefs and DAG references without any worker
// status, counters or result summaries from the Yakit display tree.
type planDefinitionNode struct {
	TaskID     string                `json:"task_id"`
	Index      string                `json:"index"`
	Name       string                `json:"name"`
	Goal       string                `json:"goal"`
	Identifier string                `json:"semantic_identifier"`
	DependsOn  []string              `json:"depends_on"`
	Subtasks   []*planDefinitionNode `json:"subtasks,omitempty"`
}

func definitionTree(n *PlanNode) *planDefinitionNode {
	d := &planDefinitionNode{TaskID: n.TaskID, Index: n.Index, Name: n.Name, Goal: n.Goal, Identifier: n.Identifier, DependsOn: append([]string{}, n.DependsOn...)}
	for _, child := range n.Subtasks {
		d.Subtasks = append(d.Subtasks, definitionTree(child))
	}
	return d
}

// PlanDefinition changes only with plan definitions or approval versions. Live
// attempts belong in PLAN STATUS, outside the cacheable SemiDynamic1 section.
func (s Snapshot) PlanDefinition() string {
	var b strings.Builder
	b.WriteString("# PLAN DEFINITION\nTask briefs and prerequisite relationships. Runtime states are in PLAN STATUS.\n")
	write := func(label string, version uint64, p *Plan, document bool) {
		if p == nil {
			return
		}
		var root PlanNode
		if json.Unmarshal(p.Tree, &root) != nil {
			return
		}
		fmt.Fprintf(&b, "\n## %s version %d\n", label, version)
		if document && strings.TrimSpace(p.Document) != "" {
			fmt.Fprintf(&b, "### Draft document\n%s\n", p.Document)
		}
		tree, _ := json.MarshalIndent(definitionTree(&root), "", "  ")
		fmt.Fprintf(&b, "```json\n%s\n```\n", tree)
		b.WriteString("Executable leaf DAG (task_id <- prerequisite task_ids):\n")
		for _, t := range p.Tasks {
			fmt.Fprintf(&b, "- %s <- [%s]\n", t.ID, strings.Join(t.DependsOn, ", "))
		}
	}
	write("Approved", s.ApprovedVersion, s.Approved, false)
	if s.Approved == nil || s.DraftVersion != s.ApprovedVersion {
		write("Draft (not approved for execution)", s.DraftVersion, s.Draft, true)
	}
	return b.String()
}
