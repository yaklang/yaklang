package coordinator_legacy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// FormatDetachedPlanTimelineContent records the old plan tree and document for approval.
func FormatDetachedPlanTimelineContent(
	coordinatorID, sessionID, reactTaskID string,
	rootTask *AiTask,
	input *aicommon.ExecutePlanInput,
) string {
	var sb strings.Builder
	sb.WriteString("detached plan published (pending user approval)\n")
	sb.WriteString(fmt.Sprintf("coordinator_id: %s\n", coordinatorID))
	if sessionID != "" {
		sb.WriteString(fmt.Sprintf("session_id: %s\n", sessionID))
	}
	if reactTaskID != "" {
		sb.WriteString(fmt.Sprintf("react_task_id: %s\n", reactTaskID))
	}
	if input != nil && strings.TrimSpace(input.PlanPayload) != "" {
		sb.WriteString(fmt.Sprintf("plan_request_payload: %s\n", strings.TrimSpace(input.PlanPayload)))
	}

	if rootTask != nil {
		if name := strings.TrimSpace(rootTask.Name); name != "" {
			sb.WriteString(fmt.Sprintf("\n# %s\n", name))
		}
		if goal := strings.TrimSpace(rootTask.Goal); goal != "" {
			sb.WriteString(fmt.Sprintf("main_task_goal: %s\n", goal))
		}
		if subs := rootTask.Subtasks; len(subs) > 0 {
			sb.WriteString("\n## plan_tasks\n")
			appendDetachedPlanTaskLines(&sb, subs, 0)
		}
	}

	if input != nil {
		if document := strings.TrimSpace(input.PlanDocument); document != "" {
			sb.WriteString("\n## plan_document\n")
			sb.WriteString(document)
			sb.WriteRune('\n')
		}
		if planData := strings.TrimSpace(input.PlanData); planData != "" {
			sb.WriteString("\n## plan_data\n")
			sb.WriteString(planData)
			sb.WriteRune('\n')
		}
	}
	return strings.TrimSpace(sb.String())
}

func appendDetachedPlanTaskLines(sb *strings.Builder, tasks []*AiTask, depth int) {
	indent := strings.Repeat("  ", depth)
	for i, task := range tasks {
		if task == nil {
			continue
		}
		name := strings.TrimSpace(task.Name)
		if name == "" {
			name = fmt.Sprintf("subtask-%d", i+1)
		}
		sb.WriteString(fmt.Sprintf("%s- %s\n", indent, name))
		if goal := strings.TrimSpace(task.Goal); goal != "" {
			sb.WriteString(fmt.Sprintf("%s  goal: %s\n", indent, goal))
		}
		if len(task.Subtasks) > 0 {
			appendDetachedPlanTaskLines(sb, task.Subtasks, depth+1)
		}
	}
}

// Decode client/storage data without invoking AiTask.UnmarshalJSON, which
// expects an initialized Coordinator even on nested children. The legacy
// builder below is the sole owner of executable task initialization.
type legacyDetachedTask struct {
	Name       string                `json:"name"`
	Goal       string                `json:"goal"`
	Identifier string                `json:"semantic_identifier"`
	DependsOn  []string              `json:"depends_on"`
	Subtasks   []*legacyDetachedTask `json:"subtasks"`
}

func (task *legacyDetachedTask) planParams(root bool) (map[string]any, error) {
	if task == nil || strings.TrimSpace(task.Name) == "" {
		return nil, errors.New("plan tree contains a null or unnamed task")
	}
	nameKey, goalKey, identifierKey, childrenKey := "subtask_name", "subtask_goal", "subtask_identifier", "sub_subtasks"
	if root {
		nameKey, goalKey, identifierKey, childrenKey = "main_task", "main_task_goal", "main_task_identifier", "tasks"
	}
	params := map[string]any{nameKey: task.Name, goalKey: task.Goal}
	if task.Identifier != "" {
		params[identifierKey] = task.Identifier
	}
	if len(task.DependsOn) > 0 {
		params["depends_on"] = task.DependsOn
	}
	children := make([]map[string]any, 0, len(task.Subtasks))
	for _, child := range task.Subtasks {
		item, err := child.planParams(false)
		if err != nil {
			return nil, err
		}
		children = append(children, item)
	}
	params[childrenKey] = children
	if root {
		params["@action"] = "plan"
	}
	return params, nil
}

// normalizePlanData accepts both model output and the task tree stored or
// approved by Yakit. All execution paths must use the same safe DTO conversion.
func normalizePlanData(planData string) (string, error) {
	var wire map[string]json.RawMessage
	if json.Unmarshal([]byte(planData), &wire) == nil && wire["name"] != nil {
		var tree legacyDetachedTask
		if err := json.Unmarshal([]byte(planData), &tree); err != nil {
			return "", err
		}
		params, err := tree.planParams(true)
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(params)
		if err != nil {
			return "", err
		}
		planData = string(data)
	}
	return planData, nil
}
