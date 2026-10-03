package coordinator

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
func (s Snapshot) PlanDefinition() string {
	if s.Plan == nil {
		return ""
	}
	var root PlanNode
	if json.Unmarshal(s.Plan.Tree, &root) != nil {
		return ""
	}
	tree, _ := json.MarshalIndent(definitionTree(&root), "", "  ")
	var b strings.Builder
	fmt.Fprintf(&b, "# PLAN DEFINITION\n当前嵌套任务定义（只含任务书和依赖）：\n```json\n%s\n```\n派生叶任务 DAG（task_id <- 前置 task_id）：\n", tree)
	for _, t := range s.Plan.Tasks {
		fmt.Fprintf(&b, "- %s <- [%s]\n", t.ID, strings.Join(t.DependsOn, ", "))
	}
	return b.String()
}
