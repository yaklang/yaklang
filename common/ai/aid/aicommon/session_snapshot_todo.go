package aicommon

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// SessionSnapshotTodoSummary contains session-wide counts without treating a
// task-local todo ID (such as todo-1) as a session-wide identity.
type SessionSnapshotTodoSummary struct {
	Stats  VerificationTodoStats            `json:"stats"`
	ByTask []SessionSnapshotTodoTaskSummary `json:"by_task"`
}

type SessionSnapshotTodoTaskSummary struct {
	TaskID    string                `json:"task_id"`
	TaskIndex string                `json:"task_index,omitempty"`
	Stats     VerificationTodoStats `json:"stats"`
}

// SessionTaskTodoSnapshot is a full, task-scoped projection. Operations such
// as applied_delta belong to the event stream, not to a durable snapshot.
type SessionTaskTodoSnapshot struct {
	Items         []VerificationTodoItem `json:"items"`
	Stats         VerificationTodoStats  `json:"stats"`
	OpenTodos     []TodoOpenItem         `json:"open_todos"`
	CurrentTodoID string                 `json:"current_todo_id,omitempty"`
	ClosedTodos   []TodoClosedItem       `json:"closed_todos"`
}

// The canonical state retains the per-task ID generator and revision for
// recovery. Sub-agent scopes are displayed in the session but never restored
// into their parent's separately owned prompt state.
type sessionSnapshotTodoScope struct {
	State      *TodoScopeState `json:"state"`
	IsSubAgent bool            `json:"is_sub_agent,omitempty"`
}

func cloneTodoScopeState(state *TodoScopeState) *TodoScopeState {
	if state == nil {
		return nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return nil
	}
	var copied TodoScopeState
	if json.Unmarshal(raw, &copied) != nil {
		return nil
	}
	return &copied
}

func cloneSessionTaskTodo(todo *SessionTaskTodoSnapshot) *SessionTaskTodoSnapshot {
	if todo == nil {
		return nil
	}
	raw, err := json.Marshal(todo)
	if err != nil {
		return nil
	}
	var copied SessionTaskTodoSnapshot
	if json.Unmarshal(raw, &copied) != nil {
		return nil
	}
	return &copied
}

func buildSessionTaskTodo(state *TodoScopeState) *SessionTaskTodoSnapshot {
	result := &SessionTaskTodoSnapshot{
		Items: []VerificationTodoItem{}, OpenTodos: []TodoOpenItem{}, ClosedTodos: []TodoClosedItem{},
	}
	if state == nil {
		return result
	}
	store := &VerificationTodoStore{Scopes: []*TodoScopeState{state}}
	result.Items = store.SnapshotItemsByScope(state.scope())
	result.Stats = store.StatsByScope(state.scope())
	result.OpenTodos, result.CurrentTodoID, result.ClosedTodos = store.CanonicalSnapshot(state.scope())
	if result.Items == nil {
		result.Items = []VerificationTodoItem{}
	}
	lifecycle := TodoListUpdatePayload{
		Items: result.Items, OpenTodos: result.OpenTodos,
		CurrentTodoID: result.CurrentTodoID, ClosedTodos: result.ClosedTodos,
	}
	enrichPayloadLifecycle(&lifecycle)
	return result
}

func todoStatsForTask(task *SessionTaskSnapshot) VerificationTodoStats {
	if task != nil && task.Todo != nil {
		return task.Todo.Stats
	}
	return VerificationTodoStats{}
}

func (s *sessionSnapshotDocumentStore) taskTodoLocked(taskID string) *SessionTaskTodoSnapshot {
	if scope := s.document.TodoScopes[taskID]; scope != nil {
		return buildSessionTaskTodo(scope.State)
	}
	return buildSessionTaskTodo(nil)
}

func (s *sessionSnapshotDocumentStore) todoSummaryLocked() *SessionSnapshotTodoSummary {
	result := &SessionSnapshotTodoSummary{ByTask: make([]SessionSnapshotTodoTaskSummary, 0, len(s.document.TodoScopes))}
	for taskID, scope := range s.document.TodoScopes {
		if scope == nil || scope.State == nil {
			continue
		}
		stats := buildSessionTaskTodo(scope.State).Stats
		result.ByTask = append(result.ByTask, SessionSnapshotTodoTaskSummary{
			TaskID: taskID, TaskIndex: scope.State.TaskIndex, Stats: stats,
		})
		result.Stats.Pending += stats.Pending
		result.Stats.Doing += stats.Doing
		result.Stats.Done += stats.Done
		result.Stats.Deleted += stats.Deleted
		result.Stats.Skipped += stats.Skipped
	}
	sort.Slice(result.ByTask, func(i, j int) bool { return result.ByTask[i].TaskID < result.ByTask[j].TaskID })
	return result
}

func (s *sessionSnapshotDocumentStore) persistLocked(c *Config) {
	if s.sessionID == "" || c.GetDB() == nil || s.document.Revision <= s.lastPersistedRev {
		return
	}
	s.document.SchemaVersion = SessionSnapshotDocumentSchemaVersion
	raw, err := json.Marshal(&s.document)
	if err != nil {
		log.Warnf("encode session snapshot document failed: %v", err)
	} else if _, err := yakit.EnsureAISessionMeta(c.GetDB(), s.sessionID); err != nil {
		log.Warnf("ensure session metadata for snapshot failed: %v", err)
	} else if err := yakit.UpdateAISessionMetaSnapshot(c.GetDB(), s.sessionID, string(raw)); err != nil {
		log.Warnf("persist session snapshot document failed: %v", err)
	} else {
		s.lastPersistedRev = s.document.Revision
	}
}

// RecordSessionSnapshotTodo persists effective changes independently of event
// emission, including when no emitter is installed or its debounce is pending.
func (c *Config) RecordSessionSnapshotTodo(task AIStatefulTask, scope VerificationTodoScope) {
	if c == nil || task == nil {
		return
	}
	scope = scope.normalize()
	if scope.TaskID == "" || scope.TaskID != strings.TrimSpace(task.GetId()) {
		return
	}
	state := c.GetSessionPromptState().SnapshotVerificationTodoScopeState(scope)
	if state == nil {
		return
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureLoadedLocked(c)
	if previous := store.document.TodoScopes[scope.TaskID]; previous != nil && previous.State != nil && previous.State.Revision >= state.Revision {
		return
	}
	store.document.TodoScopes[scope.TaskID] = &sessionSnapshotTodoScope{State: state, IsSubAgent: task.IsSubAgent()}
	if entry := store.document.Tasks[scope.TaskID]; entry != nil && !entry.IsFinal {
		entry.Todo = buildSessionTaskTodo(state)
		entry.Revision++
		entry.UpdatedAt = nowTs()
	}
	store.document.Revision++
	store.document.UpdatedAt = nowTs()
	if store.document.Session != nil {
		store.document.Session.Todo = store.todoSummaryLocked()
		store.document.Session.Tasks = store.taskSummariesLocked()
		store.document.Session.Revision = store.document.Revision
		store.document.Session.UpdatedAt = store.document.UpdatedAt
	}
	store.persistLocked(c)
}

// RestoreSessionSnapshotTodos merges only missing task scopes. Plan child
// configs share the parent's prompt state; forked sub-agents restore only their
// own scope and must not import their parent's or sibling's TODO work set.
func (c *Config) RestoreSessionSnapshotTodos(task AIStatefulTask) {
	if c == nil || task == nil {
		return
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	store.ensureLoadedLocked(c)
	states := make([]*TodoScopeState, 0, len(store.document.TodoScopes))
	for taskID, scope := range store.document.TodoScopes {
		if scope == nil || scope.State == nil {
			continue
		}
		if task.IsSubAgent() {
			if taskID != task.GetId() || !scope.IsSubAgent {
				continue
			}
		} else if scope.IsSubAgent {
			continue
		}
		states = append(states, cloneTodoScopeState(scope.State))
	}
	store.mu.Unlock()
	c.GetSessionPromptState().MergeMissingVerificationTodoScopes(states)
}
