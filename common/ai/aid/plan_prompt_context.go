package aid

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
)

// Only persisted definition fields enter this snapshot. Execution summaries,
// status, selected node and timestamps must not invalidate the Frozen prefix.
var planExecutionRules, _ = aicommon.RenderPromptTemplate("plan-execution-rules", promptloader.MustLoad("mainloop/plan_execution_rules.txt"), nil)

type planPromptNode struct {
	TaskID    string           `json:"task_id"`
	Index     string           `json:"index"`
	Name      string           `json:"name"`
	Goal      string           `json:"goal"`
	UserInput string           `json:"task_input,omitempty"`
	DependsOn []string         `json:"depends_on,omitempty"`
	Subtasks  []planPromptNode `json:"subtasks,omitempty"`
}

func snapshotPlanPromptNode(t *AiTask) planPromptNode {
	n := planPromptNode{TaskID: t.TaskId, Index: t.Index, Name: t.Name, Goal: t.Goal,
		DependsOn: append([]string(nil), t.DependsOn...)}
	if t.AIStatefulTaskBase != nil {
		n.UserInput = t.AIStatefulTaskBase.GetUserInput()
	}
	for _, child := range t.Subtasks {
		if child != nil {
			n.Subtasks = append(n.Subtasks, snapshotPlanPromptNode(child))
		}
	}
	return n
}

type planPromptDefinition struct {
	UserQuery string         `json:"user_query"`
	Root      planPromptNode `json:"root"`
}

func (t *AiTask) planDefinitionSnapshot() (planPromptDefinition, string) {
	root := findTaskTreeRoot(t)
	if root == nil {
		return planPromptDefinition{}, ""
	}
	definition := planPromptDefinition{Root: snapshotPlanPromptNode(root)}
	definition.UserQuery = root.planPromptSourceQuery()
	raw, _ := json.Marshal(definition)
	digest := sha256.Sum256(raw)
	return definition, hex.EncodeToString(digest[:])
}

// Source query is saved with the root TaskTree so recovery never substitutes
// a synthesized task label for the original user request.
func (t *AiTask) planPromptSourceQuery() string {
	if t.planSourceUserQuery != nil {
		return *t.planSourceUserQuery
	}
	if t.Coordinator != nil && t.Coordinator.userInput != "" {
		return t.Coordinator.userInput
	}
	if t.AIStatefulTaskBase != nil {
		return t.AIStatefulTaskBase.GetOriginUserInput()
	}
	return ""
}

// GetPlanPromptVersion also binds TODO ownership to the definition being executed.
// Recovered TaskTree data deterministically regenerates the same version.
func (t *AiTask) GetPlanPromptVersion() string {
	_, version := t.planDefinitionSnapshot()
	return version
}

type planPromptTaskState struct {
	TaskID       string               `json:"task_id"`
	Status       aicommon.AITaskState `json:"status"`
	Progress     string               `json:"aggregate_progress"`
	Dependencies []string             `json:"resolved_dependencies,omitempty"`
	BlockedBy    []string             `json:"blocked_by,omitempty"`
}

// GetPlanPromptContext renders from the actual task tree/strict dependency graph.
// JSON's HTML escaping neutralizes embedded delimiter syntax in plan fields;
// the Timeline's private boundary key then gives stable, content-bound framing.
// Neither the visible version nor delimiter authenticates model instructions.
func (t *AiTask) GetPlanPromptContext() aicommon.PlanPromptContext {
	definition, version := t.planDefinitionSnapshot()
	if version == "" {
		return aicommon.PlanPromptContext{}
	}
	root := findTaskTreeRoot(t)
	states := make([]planPromptTaskState, 0)
	var walk func(*AiTask)
	walk = func(node *AiTask) {
		if node == nil {
			return
		}
		states = append(states, planPromptTaskState{TaskID: node.TaskId,
			Status: node.GetStatus(), Progress: node.GetProgressStatue()})
		for _, child := range node.Subtasks {
			walk(child)
		}
	}
	walk(root)
	graph, graphErr := buildStrictExecutableTaskGraph(root)
	if graphErr == nil {
		for i := range states {
			if node, ok := graph.Node(states[i].TaskID); ok {
				states[i].Dependencies = append([]string(nil), node.deps...)
				for _, id := range node.deps {
					if dependency, exists := graph.Node(id); exists && !dependency.IsDone() {
						states[i].BlockedBy = append(states[i].BlockedBy, id)
					}
				}
			}
		}
	}
	var graphError string
	if graphErr != nil {
		graphError = graphErr.Error()
	}
	var parentID, scopeID string
	if t.ParentTask != nil {
		parentID = t.ParentTask.TaskId
	}
	if t.AIStatefulTaskBase != nil {
		scopeID = t.GetId()
	}
	runtime, _ := json.Marshal(struct {
		PlanVersion string                `json:"plan_version"`
		CurrentTask string                `json:"current_task_id"`
		CurrentName string                `json:"current_task_name"`
		CurrentGoal string                `json:"current_task_goal"`
		ParentTask  string                `json:"parent_task_id,omitempty"`
		TodoScope   string                `json:"todo_scope_task_id"`
		Tasks       []planPromptTaskState `json:"tasks"`
		GraphError  string                `json:"dependency_graph_error,omitempty"`
	}{version, t.TaskId, t.Name, t.Goal, parentID, scopeID, states, graphError})
	frozen, _ := json.Marshal(struct {
		Version    string               `json:"plan_version"`
		Definition planPromptDefinition `json:"definition"`
	}{version, definition})
	timeline := t.CurrentTimeline()
	return aicommon.PlanPromptContext{
		Version: version, UserQuery: definition.UserQuery,
		Definition:     timeline.WrapPlanReferenceForPrompt("PLAN_DEFINITION", string(frozen)),
		RuntimeState:   timeline.WrapPlanReferenceForPrompt("PLAN_RUNTIME_STATE", string(runtime)),
		ExecutionRules: planExecutionRules,
	}
}
