package coordinator

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

var runningSessions sync.Map

func GetRunningSessions() []*Session {
	var sessions []*Session
	runningSessions.Range(func(_, value any) bool {
		sessions = append(sessions, value.(*Session))
		return true
	})
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Id < sessions[j].Id })
	return sessions
}

func CollectPlanExecutionSnapshots(reactID string, holder aicommon.AIStatefulTask) []aid.PlanExecutionRuntimeSnapshot {
	var result []aid.PlanExecutionRuntimeSnapshot
	for _, s := range GetRunningSessions() {
		owner := s.Id
		if s.parent != nil {
			owner = s.parent.Id
		}
		if owner != reactID {
			continue
		}
		state := s.Snapshot()
		p := state.Plan
		if p == nil {
			continue
		}
		var root PlanNode
		if json.Unmarshal(p.Tree, &root) != nil {
			continue
		}
		report := aid.PlanExecutionRuntimeSnapshot{CoordinatorID: s.Id, RootTaskName: root.Name, TotalTasks: len(p.Tasks), AsyncReactTaskID: s.parentTaskID}
		if holder != nil && report.AsyncReactTaskID == "" {
			report.AsyncReactTaskID = holder.GetId()
		}
		levels := map[string]int{}
		var stage func(string) int
		stage = func(id string) int {
			if level := levels[id]; level > 0 {
				return level
			}
			level := 1
			for _, dep := range state.Attempts[id].Task.DependsOn {
				if next := stage(dep) + 1; next > level {
					level = next
				}
			}
			levels[id] = level
			return level
		}
		var visit func(*PlanNode, string)
		visit = func(n *PlanNode, parent string) {
			a := state.Attempts[n.TaskID]
			entry := aid.TaskRuntimeEntry{Scope: "plan_exec", ReActID: owner, CoordinatorID: s.Id, TaskID: n.TaskID, TaskIndex: n.Index,
				Name: n.Name, Goal: n.Goal, ParentIndex: parent, Status: string(a.State), LoopName: "pe_task",
				Executing: a.State == Running || a.State == Cancelling, HasTimelineFork: false, RecentTextOutputs: []aid.TaskTextOutput{}}
			if len(n.Subtasks) > 0 {
				entry.LoopName, entry.Status = Name, string(state.Phase)
			}
			s.mu.Lock()
			timeline := s.workerTimelines[n.TaskID]
			s.mu.Unlock()
			if timeline != nil {
				entry.HasTimelineFork = true
				entry.RecentTextOutputs = aid.RecentTaskTextOutputs(timeline, 10)
			}
			if a.State == Accepted {
				report.CompletedTasks++
			}
			if entry.Executing {
				report.ActiveTaskIDs = append(report.ActiveTaskIDs, n.TaskID)
				if report.CurrentTaskID == "" {
					report.CurrentTaskID = n.TaskID
					report.CurrentStage = stage(n.TaskID)
				}
			}
			report.Tasks = append(report.Tasks, entry)
			for _, child := range n.Subtasks {
				visit(child, n.Index)
			}
		}
		visit(&root, "")
		result = append(result, report)
	}
	return result
}
