package coordinator

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// TaskSnapshot is a detached result view. Editing it cannot change execution.
type TaskSnapshot struct {
	Name, Goal, Index, TaskId      string
	TaskSummary, Summary, Progress string
	Subtasks                       []*TaskSnapshot
}

type ContextSnapshot struct {
	Query                                        string
	RootTask, CurrentTask                        *TaskSnapshot
	persistent, timeline, frozen, open, evidence string
}

func (s *Session) ContextSnapshot(query string, state Snapshot) *ContextSnapshot {
	v := &ContextSnapshot{Query: query, CurrentTask: &TaskSnapshot{}, persistent: strings.Join(s.PersistentMemory, "\n"), evidence: s.GetSessionEvidenceRendered()}
	v.timeline = s.Timeline.Dump()
	v.frozen, v.open = s.Timeline.DumpFrozenOpen()
	if state.Plan == nil {
		return v
	}
	var root PlanNode
	if json.Unmarshal(state.Plan.Tree, &root) != nil {
		return v
	}
	var visit func(*PlanNode) *TaskSnapshot
	visit = func(n *PlanNode) *TaskSnapshot {
		t := &TaskSnapshot{Name: n.Name, Goal: n.Goal, Index: n.Index, TaskId: n.TaskID}
		if a, ok := state.Attempts[n.TaskID]; ok {
			t.TaskSummary, t.Summary, t.Progress = a.Result.Summary, a.Result.Summary, string(a.State)
			if a.Result.Summary != "" {
				v.CurrentTask = t
			}
		}
		for _, child := range n.Subtasks {
			t.Subtasks = append(t.Subtasks, visit(child))
		}
		return t
	}
	v.RootTask = visit(&root)
	if state.Finished {
		v.RootTask.Progress = "completed"
	}
	var summary []string
	for _, task := range state.Plan.Tasks {
		a := state.Attempts[task.ID]
		summary = append(summary, fmt.Sprintf("%s [%s]: %s", task.Name, a.State, a.Result.Summary))
	}
	v.RootTask.TaskSummary = strings.Join(summary, "\n")
	return v
}

func (v *ContextSnapshot) OS() string   { return runtime.GOOS }
func (v *ContextSnapshot) Arch() string { return runtime.GOARCH }
func (v *ContextSnapshot) Now() string  { return time.Now().Format("2006-01-02 15:04:05") }
func (v *ContextSnapshot) PersistentMemory() string {
	return "<persistent_memory>\n" + v.persistent + "\n</persistent_memory>\n"
}
func (v *ContextSnapshot) TimelineDump() string                     { return v.timeline }
func (v *ContextSnapshot) TimelineDumpFrozenOpen() (string, string) { return v.frozen, v.open }
func (v *ContextSnapshot) Evidence() string                         { return v.evidence }
func (v *ContextSnapshot) Progress() string {
	if v.RootTask == nil {
		return ""
	}
	data, _ := json.MarshalIndent(v.RootTask, "", "  ")
	return string(data)
}

// ResultMaterial contains facts, not a second user input or mutable provider.
func (v *ContextSnapshot) ResultMaterial() string {
	return v.Progress() + "\n\n" + v.Evidence() + "\n\n" + v.TimelineDump()
}
