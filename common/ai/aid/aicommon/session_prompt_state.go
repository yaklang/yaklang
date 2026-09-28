package aicommon

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

const sessionEvidenceTokenBudget = 15000

// SessionPromptState holds session-scoped prompt rendering data that must stay
// consistent across configs sharing the same conversation.
type SessionPromptState struct {
	m sync.RWMutex

	UserInputHistory []schema.AIAgentUserInputRecord

	// evidenceJSON is a business mirror / legacy import source. Timeline owns
	// mutations, freeze boundaries and prompt rendering.
	// Persisted to DB alongside UserInputHistory under the same persistent session.
	evidenceJSON string

	// todoJSON is the session's in-memory, task-scoped TODO work set. Normal
	// ReAct actions update it; main-loop prompts project it after the Open Timeline.
	todoJSON string

	// sessionArtifactsState keeps the sealed frozen artifact snapshots for the
	// current session/workdir. It is intentionally in-memory only; persistent
	// restore can add serialization later without changing the prompt API.
	sessionArtifactsState *SessionArtifactsRenderState

	// reportedRiskStore is the session-level "已报告漏洞清单" accumulator.
	// Each time a risk is emitted via cybersecurity-risk (or any risk-emitting
	// tool), the FeedBacker callback calls AppendReportedRisk to append a
	// compact summary. The rendered block is injected into the timeline-open
	// prompt section (after PlanContext) so the model sees a machine-readable
	// list of already-reported vulnerabilities and avoids duplicate calls to
	// cybersecurity-risk.
	//
	// This store is **shared** across parent and sub-agents via ForkForSubAgent
	// (pointer aliasing, not copy). Both parent and child see the same list, and
	// a risk reported by a sub-agent is immediately visible to the parent.
	//
	// 关键词: reportedRiskStore, ReportedRiskStore, 已报告漏洞清单, 去重, 共享
	reportedRiskStore *ReportedRiskStore
}

func NewSessionPromptState() *SessionPromptState {
	return &SessionPromptState{}
}

// ForkForSubAgent returns a deep copy of the session prompt state for a
// forked sub ReAct agent. User history, evidence and render state are copied;
// the reported-risk store is deliberately shared for session-wide deduplication.
// The global verification TODO store (todoJSON) is omitted: that list is the parent
// agent's verification bookkeeping and must neither leak into a sub agent's
// prompt nor be polluted by a sub agent's todo_delta. The sub agent
// therefore starts with an empty TODO list.
//
// 关键词: ForkForSubAgent, 子 agent 隔离, 复制非 todo 状态, todoJSON 丢弃
func (s *SessionPromptState) ForkForSubAgent() *SessionPromptState {
	return s.forkForSubAgent(true)
}

// ForkForTaskOnlySubAgent omits conversation state while retaining the shared
// reporting ledger. It is a context policy, not a new authority boundary.
func (s *SessionPromptState) ForkForTaskOnlySubAgent() *SessionPromptState {
	return s.forkForSubAgent(false)
}

func (s *SessionPromptState) forkForSubAgent(inheritConversation bool) *SessionPromptState {
	if s == nil {
		return NewSessionPromptState()
	}
	s.m.Lock()
	defer s.m.Unlock()

	forked := &SessionPromptState{}
	// Share the thread-safe reporting ledger even before the first report.
	// A copied nil pointer would allow parent/children to create separate stores.
	forked.reportedRiskStore = s.getOrCreateReportedRiskStore()
	if !inheritConversation {
		return forked
	}

	if len(s.UserInputHistory) > 0 {
		forked.UserInputHistory = make([]schema.AIAgentUserInputRecord, len(s.UserInputHistory))
		copy(forked.UserInputHistory, s.UserInputHistory)
	}

	forked.evidenceJSON = s.evidenceJSON
	// todoJSON intentionally left empty: sub agents do not inherit the
	// parent's global TODO list.

	if s.sessionArtifactsState != nil {
		forked.sessionArtifactsState = s.sessionArtifactsState.Fork()
	}

	return forked
}

func (s *SessionPromptState) GetOrCreateSessionArtifactsRenderState() *SessionArtifactsRenderState {
	if s == nil {
		return NewSessionArtifactsRenderState()
	}
	s.m.Lock()
	defer s.m.Unlock()
	if s.sessionArtifactsState == nil {
		s.sessionArtifactsState = NewSessionArtifactsRenderState()
	}
	return s.sessionArtifactsState
}

func (s *SessionPromptState) GetUserInputHistory() []schema.AIAgentUserInputRecord {
	if s == nil {
		return nil
	}
	s.m.RLock()
	defer s.m.RUnlock()
	if len(s.UserInputHistory) == 0 {
		return nil
	}
	history := make([]schema.AIAgentUserInputRecord, len(s.UserInputHistory))
	copy(history, s.UserInputHistory)
	return history
}

func (s *SessionPromptState) SetUserInputHistory(history []schema.AIAgentUserInputRecord) {
	if s == nil {
		return
	}
	s.m.Lock()
	defer s.m.Unlock()
	if len(history) == 0 {
		s.UserInputHistory = nil
		return
	}
	cloned := make([]schema.AIAgentUserInputRecord, len(history))
	copy(cloned, history)
	s.UserInputHistory = cloned
}

func (s *SessionPromptState) GetPrevSessionUserInput() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	if len(s.UserInputHistory) == 0 {
		return ""
	}
	return s.UserInputHistory[len(s.UserInputHistory)-1].UserInput
}

func (s *SessionPromptState) AppendUserInputHistory(userInput string, timestamp time.Time) (string, error) {
	if s == nil {
		return schema.QuoteUserInputHistory(nil)
	}
	s.m.Lock()
	defer s.m.Unlock()
	s.UserInputHistory = append(s.UserInputHistory, schema.AIAgentUserInputRecord{
		Round:     len(s.UserInputHistory) + 1,
		Timestamp: timestamp,
		UserInput: userInput,
	})
	history := make([]schema.AIAgentUserInputRecord, len(s.UserInputHistory))
	copy(history, s.UserInputHistory)
	return schema.QuoteUserInputHistory(history)
}

func (s *SessionPromptState) GetSessionEvidence() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	return s.evidenceJSON
}

func (s *SessionPromptState) SetSessionEvidence(evidenceJSON string) {
	if s == nil {
		return
	}
	s.m.Lock()
	defer s.m.Unlock()
	s.evidenceJSON = evidenceJSON
}

// GetSessionEvidenceRendered renders the business mirror for legacy restoration.
// Prompt construction uses Timeline deltas and the frozen evidence snapshot.
func (s *SessionPromptState) GetSessionEvidenceRendered() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()

	store := UnmarshalEvidenceStore(s.evidenceJSON)
	return store.Render()
}

// GetVerificationTodo returns the raw serialized in-memory TODO state.
func (s *SessionPromptState) GetVerificationTodo() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	return s.todoJSON
}

// SetVerificationTodo replaces the in-memory TODO state with the given JSON.
func (s *SessionPromptState) SetVerificationTodo(todoJSON string) {
	if s == nil {
		return
	}
	s.m.Lock()
	defer s.m.Unlock()
	s.todoJSON = todoJSON
}

// ApplyTodoDelta applies one normal ReAct action's optional todo_delta to the
// session TODO store, then re-serializes back to todoJSON. It returns one
// result entry per delta operation so callers can render a uniform summary;
// failures carry a non-empty Reason.
//
// 关键词: ApplyTodoDelta, 增量更新, per-op 结果
func (s *SessionPromptState) ApplyTodoDelta(scope VerificationTodoScope, delta *TodoDelta) []VerificationTodoApplyResult {
	if s == nil {
		return nil
	}
	s.m.Lock()
	defer s.m.Unlock()

	store := UnmarshalVerificationTodoStore(s.todoJSON)
	results := store.ApplyTodoDelta(scope, delta)
	s.todoJSON = store.Marshal()
	return results
}

func (s *SessionPromptState) ValidateTodoDelta(scope VerificationTodoScope, delta *TodoDelta) error {
	if s == nil || delta == nil {
		return nil
	}
	s.m.RLock()
	defer s.m.RUnlock()
	results := UnmarshalVerificationTodoStore(s.todoJSON).ApplyTodoDelta(scope, cloneTodoDelta(delta))
	if detail := FormatTodoDeltaValidationError(results); detail != "" {
		return fmt.Errorf("invalid todo_delta: %s", detail)
	}
	return nil
}

// GetVerificationTodoRendered returns the plain-text TODO snapshot ready for
// loop prompt injection. It includes an explicit empty current-task state.
func (s *SessionPromptState) GetVerificationTodoRendered(currentScope VerificationTodoScope) string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.RenderWithCurrentScope(currentScope)
}

// GetVerificationTodoMarkdownDelta returns the markdown snapshot computed
// against the current session state without mutating it. Callers should
// invoke this BEFORE ApplyTodoDelta when a caller needs a non-mutating preview.
//
// 关键词: GetVerificationTodoMarkdownDelta, 预览模式, 不变更状态
func (s *SessionPromptState) GetVerificationTodoMarkdownDelta(scope VerificationTodoScope, delta *TodoDelta) string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.RenderMarkdownDelta(scope, delta)
}

func (s *SessionPromptState) SnapshotCanonicalTodos(scope VerificationTodoScope) ([]TodoOpenItem, string, []TodoClosedItem) {
	if s == nil {
		return []TodoOpenItem{}, "", []TodoClosedItem{}
	}
	s.m.RLock()
	defer s.m.RUnlock()
	return UnmarshalVerificationTodoStore(s.todoJSON).CanonicalSnapshot(scope)
}

// SnapshotVerificationTodoItems returns a copy of the current TODO items for
// consumers that need structured access (e.g. emitting structured frontend
// events).
func (s *SessionPromptState) SnapshotVerificationTodoItems() []VerificationTodoItem {
	if s == nil {
		return nil
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.SnapshotItems()
}

func (s *SessionPromptState) SnapshotVerificationTodoItemsByScope(scope VerificationTodoScope) []VerificationTodoItem {
	if s == nil {
		return nil
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.SnapshotItemsByScope(scope)
}

// GetVerificationTodoStats returns aggregated stats over the current TODO
// store.
func (s *SessionPromptState) GetVerificationTodoStats() VerificationTodoStats {
	if s == nil {
		return VerificationTodoStats{}
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.Stats()
}

func (s *SessionPromptState) GetVerificationTodoStatsByScope(scope VerificationTodoScope) VerificationTodoStats {
	if s == nil {
		return VerificationTodoStats{}
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.StatsByScope(scope)
}

func (s *SessionPromptState) HasActiveVerificationTodosByScope(scope VerificationTodoScope) bool {
	if s == nil {
		return false
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.HasActiveTodosByScope(scope)
}

func (s *SessionPromptState) ActiveVerificationTodoItemsByScope(scope VerificationTodoScope) []VerificationTodoItem {
	if s == nil {
		return nil
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.ActiveTodoItemsByScope(scope)
}

// getOrCreateReportedRiskStore lazily initializes the shared store.
func (s *SessionPromptState) getOrCreateReportedRiskStore() *ReportedRiskStore {
	if s.reportedRiskStore == nil {
		s.reportedRiskStore = NewReportedRiskStore()
	}
	return s.reportedRiskStore
}

// GetReportedRisks returns the raw serialized ReportedRiskStore JSON (no
// quoting). Suitable for DB persistence callers.
func (s *SessionPromptState) GetReportedRisks() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	if s.reportedRiskStore == nil {
		return ""
	}
	return s.reportedRiskStore.Marshal()
}

// SetReportedRisks replaces the in-memory reported-risks state with the given
// JSON payload. Used during session restore from DB.
func (s *SessionPromptState) SetReportedRisks(json string) {
	if s == nil {
		return
	}
	s.m.Lock()
	defer s.m.Unlock()
	s.reportedRiskStore = UnmarshalReportedRiskStore(json)
}

// AppendReportedRisk extracts a compact summary from the given risk and
// appends it to the reported-risks store if it is not a duplicate (same
// target + type + parameter). Returns true if a new entry was added.
//
// Called from toolcall_invoke.go FeedBacker callback whenever a json-risk
// message is emitted by a tool (e.g. cybersecurity-risk). The store is
// shared across parent and sub-agents, so a risk reported by any agent
// is visible to all.
func (s *SessionPromptState) AppendReportedRisk(risk *schema.Risk) bool {
	if s == nil || risk == nil {
		return false
	}
	s.m.Lock()
	store := s.getOrCreateReportedRiskStore()
	s.m.Unlock()
	return store.AppendFromRisk(risk)
}

// GetReportedRisksRendered returns the markdown block ready for prompt
// injection into the timeline-open section. Returns empty string when no
// risks have been reported yet, so the prompt template naturally skips the
// block.
func (s *SessionPromptState) GetReportedRisksRendered() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	store := s.reportedRiskStore
	s.m.RUnlock()
	if store == nil {
		return ""
	}
	return store.Render()
}
