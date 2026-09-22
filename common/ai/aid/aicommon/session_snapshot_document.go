package aicommon

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const SessionSnapshotDocumentSchemaVersion = 1

// SessionSnapshotTaskSummary is the lightweight task index embedded in the
// cumulative session snapshot. TaskIndex is display-only; TaskID is the stable
// lookup key.
type SessionSnapshotTaskSummary struct {
	TaskID    string `json:"task_id"`
	TaskIndex string `json:"task_index,omitempty"`
	TaskName  string `json:"task_name,omitempty"`
	Attempt   int    `json:"attempt"`
	Status    string `json:"status"`
	IsFinal   bool   `json:"is_final"`
	UpdatedAt int64  `json:"updated_at"`
}

type SessionTaskSnapshotAttempt struct {
	Attempt   int              `json:"attempt"`
	Status    string           `json:"status"`
	Revision  int64            `json:"revision"`
	UpdatedAt int64            `json:"updated_at"`
	Snapshot  *SessionSnapshot `json:"snapshot"`
}

// SessionTaskSnapshot is the latest materialized projection for one logical
// task. A terminal projection is frozen and remains available after the live
// task object has been pruned.
type SessionTaskSnapshot struct {
	TaskID           string                       `json:"task_id"`
	TaskIndex        string                       `json:"task_index,omitempty"`
	TaskName         string                       `json:"task_name,omitempty"`
	Attempt          int                          `json:"attempt"`
	Status           string                       `json:"status"`
	IsFinal          bool                         `json:"is_final"`
	Revision         int64                        `json:"revision"`
	UpdatedAt        int64                        `json:"updated_at"`
	Snapshot         *SessionSnapshot             `json:"snapshot"`
	PreviousAttempts []SessionTaskSnapshotAttempt `json:"previous_attempts,omitempty"`
}

type sessionSnapshotToolAccounting struct {
	TaskID  string `json:"task_id,omitempty"`
	Success bool   `json:"success"`
}

type sessionSnapshotFileAccounting struct {
	WriteCount int      `json:"write_count"`
	TaskIDs    []string `json:"task_ids,omitempty"`
}

// SessionSnapshotAccounting contains compact identities required to make
// cumulative counters idempotent across duplicate callbacks and restoration.
type SessionSnapshotAccounting struct {
	ToolCalls           map[string]sessionSnapshotToolAccounting    `json:"tool_calls,omitempty"`
	RuntimeTasks        map[string]string                           `json:"runtime_tasks,omitempty"`
	ModifiedFiles       map[string]sessionSnapshotFileAccounting    `json:"modified_files,omitempty"`
	BackgroundProcesses map[string]SessionSnapshotBackgroundProcess `json:"background_processes,omitempty"`
	FileWrites          int                                         `json:"file_writes"`
}

// SessionSnapshotDocument is stored as one JSON value on ai_sessions_v1. It is
// deliberately a materialized projection rather than a serialized task graph.
type SessionSnapshotDocument struct {
	SchemaVersion int                             `json:"schema_version"`
	Revision      int64                           `json:"revision"`
	UpdatedAt     int64                           `json:"updated_at"`
	Session       *SessionSnapshot                `json:"session"`
	Tasks         map[string]*SessionTaskSnapshot `json:"tasks"`
	Accounting    SessionSnapshotAccounting       `json:"accounting"`
}

type sessionSnapshotDocumentStore struct {
	mu               sync.Mutex
	loaded           bool
	sessionID        string
	document         SessionSnapshotDocument
	lastPersistedRev int64
}

func newSessionSnapshotDocument() SessionSnapshotDocument {
	return SessionSnapshotDocument{
		SchemaVersion: SessionSnapshotDocumentSchemaVersion,
		Tasks:         make(map[string]*SessionTaskSnapshot),
		Accounting: SessionSnapshotAccounting{
			ToolCalls:           make(map[string]sessionSnapshotToolAccounting),
			RuntimeTasks:        make(map[string]string),
			ModifiedFiles:       make(map[string]sessionSnapshotFileAccounting),
			BackgroundProcesses: make(map[string]SessionSnapshotBackgroundProcess),
		},
	}
}

func normalizeSessionSnapshotDocument(doc *SessionSnapshotDocument) {
	if doc.SchemaVersion <= 0 {
		doc.SchemaVersion = SessionSnapshotDocumentSchemaVersion
	}
	if doc.Tasks == nil {
		doc.Tasks = make(map[string]*SessionTaskSnapshot)
	}
	for _, task := range doc.Tasks {
		if task != nil && task.Attempt <= 0 {
			task.Attempt = 1
		}
	}
	if doc.Accounting.ToolCalls == nil {
		doc.Accounting.ToolCalls = make(map[string]sessionSnapshotToolAccounting)
	}
	if doc.Accounting.RuntimeTasks == nil {
		doc.Accounting.RuntimeTasks = make(map[string]string)
	}
	if doc.Accounting.ModifiedFiles == nil {
		doc.Accounting.ModifiedFiles = make(map[string]sessionSnapshotFileAccounting)
	}
	if doc.Accounting.BackgroundProcesses == nil {
		doc.Accounting.BackgroundProcesses = make(map[string]SessionSnapshotBackgroundProcess)
	}
	if doc.Session != nil {
		NormalizeSessionSnapshot(doc.Session)
	}
}

func (c *Config) getSessionSnapshotDocumentStore() *sessionSnapshotDocumentStore {
	if c == nil {
		return nil
	}
	if c.m == nil {
		c.m = new(sync.Mutex)
	}
	c.m.Lock()
	defer c.m.Unlock()
	if c.sessionSnapshotDocument == nil {
		c.sessionSnapshotDocument = &sessionSnapshotDocumentStore{document: newSessionSnapshotDocument()}
	}
	return c.sessionSnapshotDocument
}

func (s *sessionSnapshotDocumentStore) ensureLoadedLocked(c *Config) {
	requestedSessionID := ""
	if c != nil {
		requestedSessionID = strings.TrimSpace(c.PersistentSessionId)
	}
	if s.loaded && (s.sessionID != "" || requestedSessionID == "") {
		return
	}
	s.loaded = true
	if s.document.SchemaVersion == 0 {
		s.document = newSessionSnapshotDocument()
	}
	if c == nil {
		return
	}
	s.sessionID = requestedSessionID
	if s.sessionID == "" || c.GetDB() == nil {
		return
	}
	raw, err := yakit.GetAISessionMetaSnapshot(c.GetDB(), s.sessionID)
	if err != nil {
		log.Warnf("load session snapshot document failed: %v", err)
		return
	}
	if strings.TrimSpace(raw) == "" {
		return
	}
	var restored SessionSnapshotDocument
	if err := json.Unmarshal([]byte(raw), &restored); err != nil {
		log.Warnf("decode session snapshot document failed: %v", err)
		return
	}
	normalizeSessionSnapshotDocument(&restored)
	s.document = restored
	s.lastPersistedRev = restored.Revision
}

func (c *Config) recordSessionSnapshotToolAccounting(result *aitool.ToolResult) {
	if c == nil || result == nil {
		return
	}
	id := strings.TrimSpace(result.ToolCallID)
	if id == "" {
		return
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureLoadedLocked(c)
	if _, exists := store.document.Accounting.ToolCalls[id]; exists {
		return
	}
	taskID := c.resolveHotpatchCurrentTaskId()
	store.document.Accounting.ToolCalls[id] = sessionSnapshotToolAccounting{
		TaskID: taskID, Success: result.Success,
	}
	store.document.Accounting.RuntimeTasks[id] = taskID
}

func (c *Config) recordSessionSnapshotRuntimeAccounting(runtimeID string) {
	runtimeID = strings.TrimSpace(runtimeID)
	if c == nil || runtimeID == "" {
		return
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureLoadedLocked(c)
	store.document.Accounting.RuntimeTasks[runtimeID] = c.resolveHotpatchCurrentTaskId()
}

func appendUniqueString(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func (c *Config) recordSessionSnapshotFileAccounting(path string) {
	path = strings.TrimSpace(path)
	if c == nil || path == "" {
		return
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureLoadedLocked(c)
	entry := store.document.Accounting.ModifiedFiles[path]
	entry.WriteCount++
	if taskID := c.resolveHotpatchCurrentTaskId(); taskID != "" {
		entry.TaskIDs = appendUniqueString(entry.TaskIDs, taskID)
	}
	store.document.Accounting.ModifiedFiles[path] = entry
	store.document.Accounting.FileWrites++
}

func (c *Config) recordSessionSnapshotBackgroundProcess(process SessionSnapshotBackgroundProcess, remove bool) {
	if c == nil || strings.TrimSpace(process.ProcessID) == "" {
		return
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureLoadedLocked(c)
	if remove {
		delete(store.document.Accounting.BackgroundProcesses, process.ProcessID)
		return
	}
	store.document.Accounting.BackgroundProcesses[process.ProcessID] = process
}

func (s *sessionSnapshotDocumentStore) backgroundProcessesLocked() []SessionSnapshotBackgroundProcess {
	result := make([]SessionSnapshotBackgroundProcess, 0, len(s.document.Accounting.BackgroundProcesses))
	for _, process := range s.document.Accounting.BackgroundProcesses {
		result = append(result, process)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].StartedAt == result[j].StartedAt {
			return result[i].ProcessID < result[j].ProcessID
		}
		return result[i].StartedAt < result[j].StartedAt
	})
	return result
}

func cloneSessionSnapshot(snapshot *SessionSnapshot) *SessionSnapshot {
	if snapshot == nil {
		return nil
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	var copied SessionSnapshot
	if err := json.Unmarshal(raw, &copied); err != nil {
		return nil
	}
	NormalizeSessionSnapshot(&copied)
	return &copied
}

func taskSnapshotIdentity(task AIStatefulTask) (id, index, name string) {
	if task == nil {
		return "", "", ""
	}
	return strings.TrimSpace(task.GetId()), strings.TrimSpace(task.GetIndex()), strings.TrimSpace(task.GetName())
}

// BeginSessionSnapshotTask starts a new attempt when a terminal logical task is
// executed again. Previous attempts stay embedded in the same session field.
func (c *Config) BeginSessionSnapshotTask(task AIStatefulTask) {
	if c == nil || task == nil {
		return
	}
	taskID, taskIndex, taskName := taskSnapshotIdentity(task)
	if taskID == "" {
		return
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureLoadedLocked(c)
	existing := store.document.Tasks[taskID]
	if existing == nil || !existing.IsFinal {
		return
	}
	attempt := existing.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	history := append([]SessionTaskSnapshotAttempt(nil), existing.PreviousAttempts...)
	history = append(history, SessionTaskSnapshotAttempt{
		Attempt: attempt, Status: existing.Status, Revision: existing.Revision,
		UpdatedAt: existing.UpdatedAt, Snapshot: cloneSessionSnapshot(existing.Snapshot),
	})
	store.document.Tasks[taskID] = &SessionTaskSnapshot{
		TaskID: taskID, TaskIndex: taskIndex, TaskName: taskName,
		Attempt: attempt + 1, Status: "processing", Revision: existing.Revision + 1,
		UpdatedAt: time.Now().Unix(), PreviousAttempts: history,
	}
}

func (s *sessionSnapshotDocumentStore) buildCumulativeExecutionLocked(c *Config, latest *SessionSnapshot) *SessionSnapshotExecution {
	result := &SessionSnapshotExecution{Status: "completed"}
	if latest != nil && latest.Execution != nil {
		result.TaskName = latest.Execution.TaskName
	}
	terminalTasks := 0
	for _, task := range s.document.Tasks {
		if task == nil || task.Snapshot == nil || task.Snapshot.Execution == nil {
			continue
		}
		exec := task.Snapshot.Execution
		result.ExecutionRounds += exec.ExecutionRounds
		for _, attempt := range task.PreviousAttempts {
			if attempt.Snapshot != nil && attempt.Snapshot.Execution != nil {
				result.ExecutionRounds += attempt.Snapshot.Execution.ExecutionRounds
			}
		}
		if result.StartedAt == 0 || exec.StartedAt > 0 && exec.StartedAt < result.StartedAt {
			result.StartedAt = exec.StartedAt
		}
		if exec.EndedAt > result.EndedAt {
			result.EndedAt = exec.EndedAt
		}
		if !task.IsFinal {
			result.Status = "processing"
		} else {
			terminalTasks++
			if task.Status == "aborted" && result.Status != "processing" {
				result.Status = "aborted"
			}
		}
	}
	if len(s.document.Tasks) == 0 || terminalTasks == 0 && result.Status != "processing" {
		result.Status = "processing"
	}
	for _, call := range s.document.Accounting.ToolCalls {
		if call.Success {
			result.ToolCallSuccess++
		} else {
			result.ToolCallFailed++
		}
	}
	result.ToolCallTotal = result.ToolCallSuccess + result.ToolCallFailed
	result.ModifiedFileCount = len(s.document.Accounting.ModifiedFiles)
	if db := c.GetDB(); db != nil {
		runtimeIDs := make([]string, 0, len(s.document.Accounting.RuntimeTasks))
		for runtimeID := range s.document.Accounting.RuntimeTasks {
			runtimeIDs = append(runtimeIDs, runtimeID)
			result.HTTPFlowCount += yakit.CountHTTPFlowByRuntimeID(db, runtimeID)
		}
		if len(runtimeIDs) > 0 {
			if levels, err := yakit.CountRiskByRuntimeIds(db, runtimeIDs...); err != nil {
				log.Warnf("refresh cumulative session risk count failed: %v", err)
			} else if levels != nil {
				result.RiskLevelCount = *levels
				result.RiskCount = int(levels.Total)
			}
		}
	}
	if result.StartedAt > 0 && result.EndedAt >= result.StartedAt {
		result.ExecutionMinutes = int(time.Unix(result.EndedAt, 0).Sub(time.Unix(result.StartedAt, 0)).Minutes())
	}
	return result
}

func (s *sessionSnapshotDocumentStore) taskSummariesLocked() []SessionSnapshotTaskSummary {
	result := make([]SessionSnapshotTaskSummary, 0, len(s.document.Tasks))
	for _, task := range s.document.Tasks {
		if task == nil {
			continue
		}
		result = append(result, SessionSnapshotTaskSummary{
			TaskID: task.TaskID, TaskIndex: task.TaskIndex, TaskName: task.TaskName,
			Attempt: task.Attempt, Status: task.Status, IsFinal: task.IsFinal, UpdatedAt: task.UpdatedAt,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].UpdatedAt == result[j].UpdatedAt {
			return result[i].TaskID < result[j].TaskID
		}
		return result[i].UpdatedAt < result[j].UpdatedAt
	})
	return result
}

// MaterializeSessionSnapshot records the latest task projection, rebuilds the
// cumulative session view, and persists the complete document on AISession.
func (c *Config) MaterializeSessionSnapshot(task AIStatefulTask, latest *SessionSnapshot) *SessionSnapshot {
	if c == nil || latest == nil {
		return latest
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureLoadedLocked(c)

	now := time.Now().Unix()
	if taskID, taskIndex, taskName := taskSnapshotIdentity(task); taskID != "" {
		if existing := store.document.Tasks[taskID]; existing == nil || !existing.IsFinal {
			revision := int64(1)
			attempt := 1
			var previousAttempts []SessionTaskSnapshotAttempt
			if existing != nil {
				revision = existing.Revision + 1
				attempt = existing.Attempt
				if attempt <= 0 {
					attempt = 1
				}
				previousAttempts = existing.PreviousAttempts
			}
			status := "processing"
			if latest.Execution != nil && strings.TrimSpace(latest.Execution.Status) != "" {
				status = strings.TrimSpace(latest.Execution.Status)
			}
			store.document.Tasks[taskID] = &SessionTaskSnapshot{
				TaskID: taskID, TaskIndex: taskIndex, TaskName: taskName,
				Attempt: attempt, Status: status, IsFinal: isSessionSnapshotExecutionTerminal(status),
				Revision: revision, UpdatedAt: now, Snapshot: cloneSessionSnapshot(latest),
				PreviousAttempts: previousAttempts,
			}
		}
	}

	store.document.Revision++
	store.document.UpdatedAt = now
	session := cloneSessionSnapshot(latest)
	if session == nil {
		session = &SessionSnapshot{}
	}
	session.Revision = store.document.Revision
	session.UpdatedAt = now
	session.Execution = store.buildCumulativeExecutionLocked(c, latest)
	session.Tasks = store.taskSummariesLocked()
	session.BackgroundProcesses = store.backgroundProcessesLocked()
	NormalizeSessionSnapshot(session)
	store.document.Session = cloneSessionSnapshot(session)

	if store.sessionID != "" && c.GetDB() != nil && store.document.Revision > store.lastPersistedRev {
		if raw, err := json.Marshal(&store.document); err != nil {
			log.Warnf("encode session snapshot document failed: %v", err)
		} else if _, err := yakit.EnsureAISessionMeta(c.GetDB(), store.sessionID); err != nil {
			log.Warnf("ensure session metadata for snapshot failed: %v", err)
		} else if err := yakit.UpdateAISessionMetaSnapshot(c.GetDB(), store.sessionID, string(raw)); err != nil {
			log.Warnf("persist session snapshot document failed: %v", err)
		} else {
			store.lastPersistedRev = store.document.Revision
		}
	}
	return session
}

func (c *Config) GetSessionTaskSnapshot(taskID string) *SessionTaskSnapshot {
	taskID = strings.TrimSpace(taskID)
	if c == nil || taskID == "" {
		return nil
	}
	store := c.getSessionSnapshotDocumentStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureLoadedLocked(c)
	task := store.document.Tasks[taskID]
	if task == nil {
		return nil
	}
	raw, err := json.Marshal(task)
	if err != nil {
		return nil
	}
	var copied SessionTaskSnapshot
	if err := json.Unmarshal(raw, &copied); err != nil {
		return nil
	}
	return &copied
}
