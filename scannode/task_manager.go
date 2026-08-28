package scannode

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/utils"
)

type TaskManager struct {
	mu           sync.RWMutex
	byAttempt    map[string]*Task
	byTaskID     map[string]string
	bySubtask    map[string]map[string]struct{}
	stateMu      sync.Mutex
	shuttingDown bool
}

func newTaskManager() *TaskManager {
	return &TaskManager{
		byAttempt: make(map[string]*Task),
		byTaskID:  make(map[string]string),
		bySubtask: make(map[string]map[string]struct{}),
	}
}

func (t *TaskManager) Add(taskID string, task *Task) bool {
	if task == nil {
		return false
	}
	task.TaskId = taskID
	if strings.TrimSpace(task.AttemptID) == "" {
		task.AttemptID = taskID
	}
	_, loaded, accepted := t.LoadOrStoreAttempt(task)
	if !accepted || loaded {
		return false
	}
	task.MarkRunning()
	return true
}

// LoadOrStoreAttempt atomically claims AttemptID as the primary execution
// identity. The subtask/task indexes are secondary lookup paths used by cancel
// and legacy progress reporting; they never replace an existing attempt.
func (t *TaskManager) LoadOrStoreAttempt(task *Task) (actual *Task, loaded bool, accepted bool) {
	if t == nil || task == nil || strings.TrimSpace(task.AttemptID) == "" {
		return nil, false, false
	}
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.shuttingDown {
		return nil, false, false
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if existing, ok := t.byAttempt[task.AttemptID]; ok {
		return existing, true, true
	}
	now := time.Now().UTC()
	task.StartTimestamp = now.Unix()
	ddl, ok := task.Ctx.Deadline()
	if ok {
		task.DeadlineTimestamp = ddl.Unix()
	}
	task.MarkClaimedAt(now)
	t.byAttempt[task.AttemptID] = task
	if task.TaskId != "" {
		t.byTaskID[task.TaskId] = task.AttemptID
	}
	if task.SubtaskID != "" {
		attempts := t.bySubtask[task.SubtaskID]
		if attempts == nil {
			attempts = make(map[string]struct{})
			t.bySubtask[task.SubtaskID] = attempts
		}
		attempts[task.AttemptID] = struct{}{}
	}
	return task, false, true
}

func (t *TaskManager) BeginShutdown() {
	if t == nil {
		return
	}
	t.stateMu.Lock()
	t.shuttingDown = true
	t.mu.RLock()
	for _, task := range t.byAttempt {
		if task != nil && task.Cancel != nil {
			task.Cancel()
		}
	}
	t.mu.RUnlock()
	t.stateMu.Unlock()
}

func (t *TaskManager) WaitForEmpty(ctx context.Context) error {
	if t == nil || t.Count() == 0 {
		return nil
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if t.Count() == 0 {
				return nil
			}
		}
	}
}

func (t *TaskManager) Remove(taskID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	attemptID := t.byTaskID[taskID]
	t.removeAttemptLocked(attemptID)
	t.mu.Unlock()
}

func (t *TaskManager) RemoveAttempt(attemptID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.removeAttemptLocked(attemptID)
	t.mu.Unlock()
}

func (t *TaskManager) removeAttemptLocked(attemptID string) {
	task := t.byAttempt[attemptID]
	if task == nil {
		return
	}
	delete(t.byAttempt, attemptID)
	if task.TaskId != "" && t.byTaskID[task.TaskId] == attemptID {
		delete(t.byTaskID, task.TaskId)
	}
	if attempts := t.bySubtask[task.SubtaskID]; attempts != nil {
		delete(attempts, attemptID)
		if len(attempts) == 0 {
			delete(t.bySubtask, task.SubtaskID)
		}
	}
}

func (t *TaskManager) GetTaskById(taskID string) (*Task, error) {
	if t != nil {
		t.mu.RLock()
		attemptID := t.byTaskID[taskID]
		task := t.byAttempt[attemptID]
		t.mu.RUnlock()
		if task != nil {
			return task, nil
		}
	}
	return nil, utils.Errorf("no existed task: %s", taskID)
}

func (t *TaskManager) GetTaskByAttemptID(attemptID string) (*Task, error) {
	if t != nil {
		t.mu.RLock()
		task := t.byAttempt[attemptID]
		t.mu.RUnlock()
		if task != nil {
			return task, nil
		}
	}
	return nil, utils.Errorf("no existed attempt: %s", attemptID)
}

func (t *TaskManager) TasksBySubtask(subtaskID string) []*Task {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	attempts := t.bySubtask[subtaskID]
	result := make([]*Task, 0, len(attempts))
	for attemptID := range attempts {
		if task := t.byAttempt[attemptID]; task != nil {
			result = append(result, task)
		}
	}
	return result
}

func (t *TaskManager) Count() int {
	if t == nil {
		return 0
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.byAttempt)
}

func (t *TaskManager) Touch(taskID string) {
	task, err := t.GetTaskById(taskID)
	if err != nil {
		return
	}
	task.Touch()
}

func (t *TaskManager) TouchAttempt(attemptID string) {
	task, err := t.GetTaskByAttemptID(attemptID)
	if err != nil {
		return
	}
	task.Touch()
}

func (t *TaskManager) MarkCancelRequested(taskID string) {
	task, err := t.GetTaskById(taskID)
	if err != nil {
		return
	}
	task.MarkCancelRequested()
}

func (t *TaskManager) ActiveAttemptHeartbeats(now time.Time) []node.ActiveAttemptHeartbeat {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	t.mu.RLock()
	items := make([]node.ActiveAttemptHeartbeat, 0, len(t.byAttempt))
	for _, task := range t.byAttempt {
		item, ok := task.activeAttemptHeartbeat(now)
		if ok {
			items = append(items, item)
		}
	}
	t.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		if items[i].AttemptID == items[j].AttemptID {
			return items[i].SubtaskID < items[j].SubtaskID
		}
		return items[i].AttemptID < items[j].AttemptID
	})
	return items
}

type Task struct {
	TaskType          string
	TaskId            string
	JobID             string
	SubtaskID         string
	AttemptID         string
	Ctx               context.Context
	Cancel            context.CancelFunc
	StartTimestamp    int64
	DeadlineTimestamp int64
	cancelReason      string
	cancelReasonMu    sync.RWMutex
	status            string
	statusMu          sync.RWMutex
	lastActivityAt    time.Time
	activityMu        sync.RWMutex
	completedUnits    uint32
	totalUnits        uint32
	progressMu        sync.RWMutex
}

func (t *Task) SetCancelReason(reason string) {
	t.cancelReasonMu.Lock()
	defer t.cancelReasonMu.Unlock()
	t.cancelReason = reason
}

func (t *Task) CancelReason() string {
	t.cancelReasonMu.RLock()
	defer t.cancelReasonMu.RUnlock()
	return t.cancelReason
}

func (t *Task) MarkRunning() {
	t.MarkRunningAt(time.Now().UTC())
}

func (t *Task) MarkClaimedAt(now time.Time) {
	t.statusMu.Lock()
	t.status = "claimed"
	t.statusMu.Unlock()
	t.TouchAt(now)
}

func (t *Task) MarkRunningAt(now time.Time) {
	t.statusMu.Lock()
	t.status = "running"
	t.statusMu.Unlock()
	t.TouchAt(now)
}

func (t *Task) MarkCancelRequested() {
	t.statusMu.Lock()
	t.status = "cancel_requested"
	t.statusMu.Unlock()
	t.Touch()
}

func (t *Task) Touch() {
	t.TouchAt(time.Now().UTC())
}

func (t *Task) TouchAt(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	t.activityMu.Lock()
	t.lastActivityAt = now
	t.activityMu.Unlock()
}

func (t *Task) UpdateProgressAt(completedUnits uint32, totalUnits uint32, now time.Time) {
	if totalUnits > 0 && completedUnits > totalUnits {
		completedUnits = totalUnits
	}
	t.progressMu.Lock()
	t.completedUnits = completedUnits
	t.totalUnits = totalUnits
	t.progressMu.Unlock()
	t.TouchAt(now)
}

func (t *Task) Status() string {
	t.statusMu.RLock()
	defer t.statusMu.RUnlock()
	return t.status
}

func (t *Task) Progress() (uint32, uint32) {
	t.progressMu.RLock()
	defer t.progressMu.RUnlock()
	return t.completedUnits, t.totalUnits
}

func (t *Task) LastActivityAt(now time.Time) time.Time {
	t.activityMu.RLock()
	last := t.lastActivityAt
	t.activityMu.RUnlock()
	if last.IsZero() {
		return now
	}
	return last
}

func (t *Task) activeAttemptHeartbeat(now time.Time) (node.ActiveAttemptHeartbeat, bool) {
	if t == nil || t.AttemptID == "" {
		return node.ActiveAttemptHeartbeat{}, false
	}
	status := t.Status()
	if status == "" {
		status = "claimed"
	}
	switch status {
	case "claimed", "running", "cancel_requested":
	default:
		return node.ActiveAttemptHeartbeat{}, false
	}
	completedUnits, totalUnits := t.Progress()
	return node.ActiveAttemptHeartbeat{
		AttemptID:      t.AttemptID,
		JobID:          t.JobID,
		SubtaskID:      t.SubtaskID,
		Status:         status,
		CompletedUnits: completedUnits,
		TotalUnits:     totalUnits,
		LastActivityAt: t.LastActivityAt(now),
	}, true
}

func taskIDForSubtask(subtaskID string) string {
	return "script-task-" + subtaskID
}
