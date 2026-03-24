package aid

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type TaskDeltaOp string

const (
	TaskDeltaInsertAfter TaskDeltaOp = "insert_after"
	TaskDeltaAppend      TaskDeltaOp = "append"
	TaskDeltaRemove      TaskDeltaOp = "remove"
	TaskDeltaModify      TaskDeltaOp = "modify"
	TaskDeltaReplaceAll  TaskDeltaOp = "replace_all"
)

type TaskDelta struct {
	Op           TaskDeltaOp        `json:"op"`
	RefTaskIndex string             `json:"ref_task_index,omitempty"`
	Tasks        []TaskDeltaNewTask `json:"tasks,omitempty"`
	UpdatedName  string             `json:"updated_name,omitempty"`
	UpdatedGoal  string             `json:"updated_goal,omitempty"`
}

type TaskDeltaNewTask struct {
	SubtaskName string `json:"subtask_name"`
	SubtaskGoal string `json:"subtask_goal"`
}

func ParseTaskDeltas(param aitool.InvokeParams) []TaskDelta {
	rawDeltas, ok := param["task_deltas"]
	if !ok || rawDeltas == nil {
		return nil
	}

	deltaSlice, ok := rawDeltas.([]interface{})
	if !ok {
		return nil
	}

	var deltas []TaskDelta
	for _, raw := range deltaSlice {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		d := TaskDelta{
			Op:           TaskDeltaOp(utils.InterfaceToString(m["op"])),
			RefTaskIndex: utils.InterfaceToString(m["ref_task_index"]),
			UpdatedName:  utils.InterfaceToString(m["updated_name"]),
			UpdatedGoal:  utils.InterfaceToString(m["updated_goal"]),
		}
		if rawTasks, ok := m["tasks"]; ok {
			if tasksSlice, ok := rawTasks.([]interface{}); ok {
				for _, rt := range tasksSlice {
					if tm, ok := rt.(map[string]interface{}); ok {
						d.Tasks = append(d.Tasks, TaskDeltaNewTask{
							SubtaskName: utils.InterfaceToString(tm["subtask_name"]),
							SubtaskGoal: utils.InterfaceToString(tm["subtask_goal"]),
						})
					}
				}
			}
		}
		deltas = append(deltas, d)
	}
	return deltas
}

// ApplyTaskDeltas applies a list of delta operations to the task tree.
// It operates on the parent task's subtasks (siblings of t).
// Execution order: replace_all > remove > modify > insert_after > append.
// All ref_task_index values refer to indices as they were BEFORE any deltas are applied.
func (t *AiTask) ApplyTaskDeltas(deltas []TaskDelta) error {
	parentTask := t.ParentTask
	if parentTask == nil {
		return utils.Error("cannot apply task deltas: current task has no parent")
	}

	currentIndex := -1
	indexToPos := make(map[string]int)
	for i, sub := range parentTask.Subtasks {
		indexToPos[sub.Index] = i
		if sub.Index == t.Index {
			currentIndex = i
		}
	}
	if currentIndex == -1 {
		return utils.Error("cannot apply task deltas: current task not found in parent subtasks")
	}

	var replaceAllDeltas, removeDeltas, modifyDeltas, insertAfterDeltas, appendDeltas []TaskDelta
	for _, d := range deltas {
		switch d.Op {
		case TaskDeltaReplaceAll:
			replaceAllDeltas = append(replaceAllDeltas, d)
		case TaskDeltaRemove:
			removeDeltas = append(removeDeltas, d)
		case TaskDeltaModify:
			modifyDeltas = append(modifyDeltas, d)
		case TaskDeltaInsertAfter:
			insertAfterDeltas = append(insertAfterDeltas, d)
		case TaskDeltaAppend:
			appendDeltas = append(appendDeltas, d)
		default:
			log.Warnf("unknown task delta op: %s, skipping", d.Op)
		}
	}

	// replace_all takes precedence over everything else
	if len(replaceAllDeltas) > 0 {
		d := replaceAllDeltas[0]
		if len(d.Tasks) == 0 {
			return utils.Error("replace_all delta has no tasks")
		}
		parentTask.Subtasks = parentTask.Subtasks[:currentIndex+1]
		for _, nt := range d.Tasks {
			newTask := t.Coordinator.generateAITaskWithName(nt.SubtaskName, nt.SubtaskGoal)
			newTask.ParentTask = parentTask
			parentTask.Subtasks = append(parentTask.Subtasks, newTask)
			t.Coordinator.EmitInfo("delta replace_all: new task %q", nt.SubtaskName)
		}
		t.propagateAndReindex("task delta replace_all applied")
		return nil
	}

	// remove: sort by position descending to avoid index shift
	sort.Slice(removeDeltas, func(i, j int) bool {
		pi := indexToPos[removeDeltas[i].RefTaskIndex]
		pj := indexToPos[removeDeltas[j].RefTaskIndex]
		return pi > pj
	})
	for _, d := range removeDeltas {
		pos, ok := indexToPos[d.RefTaskIndex]
		if !ok {
			log.Warnf("delta remove: ref_task_index %q not found, skipping", d.RefTaskIndex)
			continue
		}
		if pos <= currentIndex {
			log.Warnf("delta remove: cannot remove completed/current task %q, skipping", d.RefTaskIndex)
			continue
		}
		target := parentTask.Subtasks[pos]
		if target.executed() || target.GetStatus() == aicommon.AITaskState_Processing {
			log.Warnf("delta remove: task %q is executed or processing, skipping", d.RefTaskIndex)
			continue
		}
		parentTask.Subtasks = append(parentTask.Subtasks[:pos], parentTask.Subtasks[pos+1:]...)
		t.Coordinator.EmitInfo("delta remove: removed task %q", d.RefTaskIndex)
	}

	// rebuild indexToPos after removes
	indexToPos = make(map[string]int)
	for i, sub := range parentTask.Subtasks {
		indexToPos[sub.Index] = i
		if sub.Index == t.Index {
			currentIndex = i
		}
	}

	// modify
	for _, d := range modifyDeltas {
		pos, ok := indexToPos[d.RefTaskIndex]
		if !ok {
			log.Warnf("delta modify: ref_task_index %q not found, skipping", d.RefTaskIndex)
			continue
		}
		if pos <= currentIndex {
			log.Warnf("delta modify: cannot modify completed/current task %q, skipping", d.RefTaskIndex)
			continue
		}
		target := parentTask.Subtasks[pos]
		if target.executed() || target.GetStatus() == aicommon.AITaskState_Processing {
			log.Warnf("delta modify: task %q is executed or processing, skipping", d.RefTaskIndex)
			continue
		}
		if d.UpdatedName != "" {
			target.Name = d.UpdatedName
		}
		if d.UpdatedGoal != "" {
			target.Goal = d.UpdatedGoal
		}
		t.Coordinator.EmitInfo("delta modify: updated task %q", d.RefTaskIndex)
	}

	// insert_after: sort by original position ascending; track offset for correct insertion
	sort.Slice(insertAfterDeltas, func(i, j int) bool {
		pi := indexToPos[insertAfterDeltas[i].RefTaskIndex]
		pj := indexToPos[insertAfterDeltas[j].RefTaskIndex]
		return pi < pj
	})
	offset := 0
	for _, d := range insertAfterDeltas {
		pos, ok := indexToPos[d.RefTaskIndex]
		if !ok {
			log.Warnf("delta insert_after: ref_task_index %q not found, skipping", d.RefTaskIndex)
			continue
		}
		if len(d.Tasks) == 0 {
			log.Warnf("delta insert_after: no tasks to insert for ref %q, skipping", d.RefTaskIndex)
			continue
		}
		insertPos := pos + offset + 1
		var newTasks []*AiTask
		for _, nt := range d.Tasks {
			newTask := t.Coordinator.generateAITaskWithName(nt.SubtaskName, nt.SubtaskGoal)
			newTask.ParentTask = parentTask
			newTasks = append(newTasks, newTask)
			t.Coordinator.EmitInfo("delta insert_after %q: new task %q", d.RefTaskIndex, nt.SubtaskName)
		}
		tail := make([]*AiTask, len(parentTask.Subtasks[insertPos:]))
		copy(tail, parentTask.Subtasks[insertPos:])
		parentTask.Subtasks = append(parentTask.Subtasks[:insertPos], newTasks...)
		parentTask.Subtasks = append(parentTask.Subtasks, tail...)
		offset += len(newTasks)
	}

	// append
	for _, d := range appendDeltas {
		for _, nt := range d.Tasks {
			newTask := t.Coordinator.generateAITaskWithName(nt.SubtaskName, nt.SubtaskGoal)
			newTask.ParentTask = parentTask
			parentTask.Subtasks = append(parentTask.Subtasks, newTask)
			t.Coordinator.EmitInfo("delta append: new task %q", nt.SubtaskName)
		}
	}

	t.propagateAndReindex("task delta applied")
	return nil
}

func (t *AiTask) propagateAndReindex(reason string) {
	if t.Coordinator == nil {
		return
	}
	t.Coordinator.standardizeTaskTreeAndNotify(t.ParentTask, reason)
}

// GetPendingSiblingTasksInfo returns a text description of pending sibling tasks
// that come after the current task in the parent's subtask list.
func (t *AiTask) GetPendingSiblingTasksInfo() string {
	if t.ParentTask == nil {
		return ""
	}

	foundCurrent := false
	var lines []string
	for _, sub := range t.ParentTask.Subtasks {
		if sub.Index == t.Index {
			foundCurrent = true
			continue
		}
		if !foundCurrent {
			continue
		}
		if sub.executed() {
			continue
		}
		line := fmt.Sprintf("  %s. %q (Goal: %s)", sub.Index, sub.Name, sub.Goal)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "(no pending tasks)"
	}
	return strings.Join(lines, "\n")
}
