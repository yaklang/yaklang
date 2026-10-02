package coordinator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Progress preserves Yakit's status fields. Native scheduler state is owned
// here, not added to the legacy PlanAndExecProgress or runtime.
type Progress struct {
	PlanEngine       string    `json:"plan_engine"`
	CoordinatorState *Snapshot `json:"coordinator_state"`
	ReactTaskID      string    `json:"react_task_id,omitempty"`
	PlanPayload      string    `json:"plan_payload,omitempty"`
	PlanDocument     string    `json:"plan_document,omitempty"`
	TotalTasks       int       `json:"total_tasks"`
	CompletedTasks   int       `json:"completed_tasks"`
	SkippedTasks     int       `json:"skipped_tasks"`
	AbortedTasks     int       `json:"aborted_tasks"`
	TotalStages      int       `json:"total_stages"`
	CompletedStages  int       `json:"completed_stages"`
	CurrentStage     int       `json:"current_stage"`
	CurrentIndex     int       `json:"current_index"`
	CurrentTaskID    string    `json:"current_task_id"`
	CurrentTask      string    `json:"current_task"`
	CurrentGoal      string    `json:"current_goal"`
	ActiveTaskIDs    []string  `json:"active_task_ids,omitempty"`
	Phase            string    `json:"phase"`
	UpdatedAt        int64     `json:"updated_at"`
}

func (s *Session) Changed(snapshot Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := snapshot.Approved
	if p == nil {
		p = snapshot.Draft
	}
	if p == nil {
		s.last = snapshot
		return
	}
	var root PlanNode
	if err := json.Unmarshal(p.Tree, &root); err != nil {
		s.stateErr = err
		return
	}
	progress := Progress{PlanEngine: Name, CoordinatorState: &snapshot, ReactTaskID: s.parentTaskID, PlanPayload: s.query, PlanDocument: p.Document, TotalTasks: len(p.Tasks), Phase: "NotCompleted", UpdatedAt: time.Now().Unix()}
	if snapshot.Approved == nil {
		progress.Phase = aicommon.PlanExecPhaseDetachedPendingApproval
	} else if s.planningOnly {
		progress.Phase = "plan_ready"
	}
	if snapshot.Finished && !s.PlanningOnly() {
		progress.Phase = "Completed"
		root.Progress = "completed"
	}
	stages := map[string]int{}
	var stage func(string) int
	stage = func(id string) int {
		if n := stages[id]; n > 0 {
			return n
		}
		n := 1
		for _, dep := range snapshot.Attempts[id].Task.DependsOn {
			if v := stage(dep) + 1; v > n {
				n = v
			}
		}
		stages[id] = n
		return n
	}
	stageDone := map[int]bool{}
	for _, task := range p.Tasks {
		n := stage(task.ID)
		if n > progress.TotalStages {
			progress.TotalStages = n
		}
		if _, ok := stageDone[n]; !ok {
			stageDone[n] = true
		}
		a := snapshot.Attempts[task.ID]
		if a.State != Accepted && a.State != Cancelled {
			stageDone[n] = false
		}
	}
	for n := 1; n <= progress.TotalStages; n++ {
		if stageDone[n] {
			progress.CompletedStages++
		} else if progress.CurrentStage == 0 {
			progress.CurrentStage = n
		}
	}
	order := map[string]int{}
	for i, t := range p.Tasks {
		order[t.ID] = i
	}
	root.walk(func(n *PlanNode) {
		if len(n.Subtasks) > 0 {
			return
		}
		a := snapshot.Attempts[n.TaskID]
		old := s.last.Attempts[n.TaskID]
		n.Summary, n.TaskSummary, n.ShortSummary = a.Result.Summary, a.Result.Summary, a.Result.Summary
		switch a.State {
		case Running, Cancelling, AwaitingReview:
			n.Progress = "processing"
			progress.ActiveTaskIDs = append(progress.ActiveTaskIDs, n.TaskID)
			if progress.CurrentTaskID == "" {
				progress.CurrentTaskID, progress.CurrentTask, progress.CurrentGoal = n.TaskID, n.Name, n.Goal
				progress.CurrentIndex = order[n.TaskID]
			}
		case Accepted:
			n.Progress = "completed"
			progress.CompletedTasks++
		case Cancelled:
			n.Progress = "skipped"
			progress.SkippedTasks++
		case Failed, Rejected:
			n.Progress = "aborted"
			progress.AbortedTasks++
		default:
			n.Progress = ""
		}
		payload := map[string]any{"index": n.Index, "name": n.Name, "goal": n.Goal, "task_id": n.TaskID}
		if task := s.tasks[n.TaskID]; task != nil {
			payload["task_uuid"] = task.GetUUID()
		}
		if a.State == Running && (old.ID != a.ID || old.State != a.State) {
			task := aicommon.NewStatefulTaskBase(n.TaskID, fmt.Sprintf("Task: %s\nGoal: %s", n.Name, n.Goal), s.GetContext(), s.GetEmitter(), true)
			s.tasks[n.TaskID] = task
			payload["task_uuid"] = task.GetUUID()
			if !s.opened[n.TaskID] {
				s.EmitStructured("system", map[string]any{"type": "push_task", "task": payload})
				s.opened[n.TaskID] = true
			} else if s.closed[n.TaskID] && s.parent != nil {
				s.parent.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "react_task_status_changed", map[string]any{"react_task_id": n.TaskID, "react_task_status": "processing"})
			}
			s.closed[n.TaskID] = false
		}
		if a.State != old.State || a.ID != old.ID || a.ReviewReason != old.ReviewReason {
			s.EmitTextMarkdownStreamEvent("coordinator-task", strings.NewReader(fmt.Sprintf("%s %s · %s (attempt %d)\n%s\n%s", n.Index, n.Name, a.State, a.ID, a.Result.Summary, a.ReviewReason)), n.Index)
			if (a.State == Accepted || a.State == Cancelled) && s.opened[n.TaskID] && !s.closed[n.TaskID] {
				s.EmitStructured("system", map[string]any{"type": "update_task_status", "task": map[string]any{"index": n.Index, "name": n.Name, "goal": n.Goal, "summary": n.Summary, "long_summary": n.LongSummary}})
				payload["task_status"] = n.Progress
				s.EmitStructured("system", map[string]any{"type": "pop_task", "task": payload})
				s.closed[n.TaskID] = true
			}
		}
	})
	if snapshot.Approved != nil && snapshot.ApprovedVersion != s.last.ApprovedVersion && strings.TrimSpace(p.Document) != "" {
		s.AppendFrozenBlockPartition("plan_document", "Plan Document", fmt.Sprintf("Approved PLAN version %d\n%s", snapshot.ApprovedVersion, p.Document), aicommon.PlanDocumentFrozenPartitionOrder)
	}
	s.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{"root_task": root})
	s.lastTree, _ = json.Marshal(root)
	if s.PersistentSessionId != "" && s.GetDB() != nil {
		tree, err := json.Marshal(root)
		if err == nil {
			var raw []byte
			raw, err = json.Marshal(progress)
			if err == nil {
				err = yakit.CreateOrUpdateAISessionPlanAndExec(s.GetDB(), &schema.AISessionPlanAndExec{SessionID: s.PersistentSessionId, CoordinatorID: s.Id, TaskTree: string(tree), TaskProgress: string(raw)})
			}
		}
		if err != nil {
			s.stateErr = err
			s.EmitError("save coordinator state: %v", err)
		}
	}
	s.last = snapshot
}

func (s *Session) registerControls() {
	s.InputEventManager.RegisterSyncCallback(aicommon.SYNC_TYPE_PLAN, func(e *ypb.AIInputEvent) error {
		s.mu.Lock()
		tree := append(json.RawMessage(nil), s.lastTree...)
		s.mu.Unlock()
		payload := map[string]any{}
		if len(tree) > 0 {
			payload["root_task"] = tree
		}
		s.EmitSyncJSON(schema.EVENT_TYPE_PLAN, "system", payload, e.SyncID)
		return nil
	})
	s.InputEventManager.RegisterSyncCallback(aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN, func(e *ypb.AIInputEvent) error { return s.control(e, false) })
	s.InputEventManager.RegisterSyncCallback(aicommon.SYNC_TYPE_REDO_SUBTASK_IN_PLAN, func(e *ypb.AIInputEvent) error { return s.control(e, true) })
}

func (s *Session) control(event *ypb.AIInputEvent, retry bool) error {
	var p struct {
		ID      string `json:"subtask_id"`
		Index   string `json:"subtask_index"`
		Reason  string `json:"reason"`
		Message string `json:"user_message"`
	}
	err := json.Unmarshal([]byte(event.SyncJsonInput), &p)
	snapshot := s.controller.Snapshot()
	if p.ID == "" {
		for id, a := range snapshot.Attempts {
			if a.Task.Index == p.Index {
				p.ID = id
				break
			}
		}
	}
	if err == nil && p.ID == "" {
		err = fmt.Errorf("subtask_id or subtask_index is required")
	}
	if retry && p.Message == "" {
		err = fmt.Errorf("user_message is required")
	}
	if err == nil && retry && snapshot.Attempts[p.ID].State != Accepted {
		err = fmt.Errorf("only completed plan tasks can be redone through this control")
	}
	if err == nil && !retry && snapshot.Attempts[p.ID].State == Accepted {
		err = fmt.Errorf("completed plan tasks cannot be skipped")
	}
	if err == nil {
		reason := p.Reason
		if retry {
			reason = p.Message
		}
		if reason == "" {
			reason = "User requested skipping this plan task."
		}
		s.AppendUserInputHistory(reason, time.Now())
		s.controller.Wake()
		if retry {
			a, ok := snapshot.Attempts[p.ID]
			if !ok {
				err = fmt.Errorf("unknown task")
			} else {
				_, err = s.controller.RetryTask(p.ID, a.ID, reason)
			}
		} else {
			err = s.controller.CancelTasks([]string{p.ID}, reason)
		}
	}
	node := "skip_subtask_in_plan"
	if retry {
		node = "redo_subtask_in_plan"
	}
	ack := func(err error) {
		payload := map[string]any{"success": err == nil, "subtask_id": p.ID}
		if err != nil {
			payload["error"] = err.Error()
		}
		s.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, node, payload, event.SyncID)
	}
	if err != nil || retry {
		ack(err)
		return nil
	}
	// Preserve the old skip acknowledgement: send success only when the owned
	// worker has stopped, not merely when its cancellation has been requested.
	go func() {
		for {
			result, err := s.controller.WaitTasks(s.GetContext(), []string{p.ID}, 30*time.Second)
			if err != nil {
				ack(err)
				return
			}
			if len(result.Tasks) > 0 && result.Tasks[0].State != Cancelling && result.Tasks[0].State != Running {
				ack(nil)
				return
			}
		}
	}()
	return nil
}
