package aicommon

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const sessionEvidenceTokenBudget = 15000

// SessionPromptState holds session-scoped prompt rendering data that must stay
// consistent across configs sharing the same conversation.
type SessionPromptState struct {
	m sync.RWMutex

	UserInputHistory []schema.AIAgentUserInputRecord

	// evidenceJSON stores the serialized EvidenceStore JSON for session-level evidence.
	// Persisted to DB alongside UserInputHistory under the same persistent session.
	evidenceJSON string

	// todoJSON stores the serialized VerificationTodoStore JSON for the global
	// task-scoped TODO work set maintained by normal ReAct actions. The list is rendered
	// into every loop prompt (timeline-open section, right after
	// SessionEvidence) so the model can see its own pending TODOs on every
	// iteration, not only at Verify checkpoints.
	//
	// 关键词: todoJSON, VerificationTodoStore 序列化, SessionEvidence 同构,
	//        全局 TODO 持久态
	todoJSON string

	// sessionArtifactsState keeps the sealed frozen artifact snapshots for the
	// current session/workdir. It is intentionally in-memory only; persistent
	// restore can add serialization later without changing the prompt API.
	sessionArtifactsState *SessionArtifactsRenderState

	// sessionEvidenceState keeps the frozen evidence snapshot used to render
	// frozen/open evidence blocks under a timeline frozen cutoff.
	sessionEvidenceState *SessionEvidenceRenderState

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
// forked sub ReAct agent. Every field is copied so the sub agent owns an
// independent state that cannot race with (or mutate) the parent's, EXCEPT
// the global verification TODO store (todoJSON): that list is the parent
// agent's verification bookkeeping and must neither leak into a sub agent's
// prompt nor be polluted by a sub agent's todo_delta. The sub agent
// therefore starts with an empty TODO list.
//
// 关键词: ForkForSubAgent, 子 agent 隔离, 复制非 todo 状态, todoJSON 丢弃
func (s *SessionPromptState) ForkForSubAgent() *SessionPromptState {
	if s == nil {
		return NewSessionPromptState()
	}
	s.m.RLock()
	defer s.m.RUnlock()

	forked := &SessionPromptState{}

	if len(s.UserInputHistory) > 0 {
		forked.UserInputHistory = make([]schema.AIAgentUserInputRecord, len(s.UserInputHistory))
		copy(forked.UserInputHistory, s.UserInputHistory)
	}

	forked.evidenceJSON = s.evidenceJSON
	// reportedRiskStore is shared (pointer-aliased) between parent and child:
	// risks reported by a sub-agent are immediately visible to the parent and
	// vice versa. The store is internally thread-safe (sync.Mutex), so
	// concurrent access from parent and child agents is safe.
	forked.reportedRiskStore = s.reportedRiskStore
	// todoJSON intentionally left empty: sub agents do not inherit the
	// parent's global TODO list.

	if s.sessionArtifactsState != nil {
		forked.sessionArtifactsState = s.sessionArtifactsState.Fork()
	}
	if s.sessionEvidenceState != nil {
		forked.sessionEvidenceState = s.sessionEvidenceState.Fork()
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
	s.sessionEvidenceState = nil
}

// ApplySessionEvidenceOps deserializes the current evidence store, applies
// the operations, shrinks to token budget, serializes back, and returns
// the quoted string suitable for DB persistence.
func (s *SessionPromptState) ApplySessionEvidenceOps(ops []EvidenceOperation) string {
	if s == nil {
		return ""
	}
	s.m.Lock()
	defer s.m.Unlock()

	store := UnmarshalEvidenceStore(s.evidenceJSON)
	store.ApplyOperations(ops)
	shrinkEvidenceStoreWithStateToTokenBudget(store, s.sessionEvidenceState, sessionEvidenceTokenBudget)
	s.evidenceJSON = store.Marshal()
	return codec.StrConvQuote(s.evidenceJSON)
}

func (s *SessionPromptState) quoteEvidence(raw string) string {
	return codec.StrConvQuote(raw)
}

// GetSessionEvidenceRendered returns markdown text ready for prompt injection.
func (s *SessionPromptState) GetSessionEvidenceRendered() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()

	store := UnmarshalEvidenceStore(s.evidenceJSON)
	return store.Render()
}

func (s *SessionPromptState) GetSessionEvidenceFrozenOpenBlocks(frozenTimeUnix int64, openNonce string) SessionEvidencePromptBlocks {
	if s == nil {
		return SessionEvidencePromptBlocks{}
	}
	s.m.Lock()
	defer s.m.Unlock()

	store := UnmarshalEvidenceStore(s.evidenceJSON)
	if s.sessionEvidenceState == nil {
		s.sessionEvidenceState = NewSessionEvidenceRenderState()
	}

	blocks := RenderSessionEvidenceFrozenOpen(s.sessionEvidenceState, store, frozenTimeUnix)
	rendered := renderSessionEvidencePromptBlocks(blocks, openNonce)
	for len(store.Items) > 1 && TokenCountExceeds(joinSessionEvidencePromptBlocks(rendered), sessionEvidenceTokenBudget) {
		trimmed := store.Items[0]
		store.Items = store.Items[1:]
		pruneSessionEvidenceFrozenItem(s.sessionEvidenceState, trimmed.ID)
		blocks = RenderSessionEvidenceFrozenOpen(s.sessionEvidenceState, store, frozenTimeUnix)
		rendered = renderSessionEvidencePromptBlocks(blocks, openNonce)
	}
	s.evidenceJSON = store.Marshal()
	return rendered
}

func shrinkEvidenceStoreWithStateToTokenBudget(store *EvidenceStore, state *SessionEvidenceRenderState, budget int) {
	if store == nil || budget <= 0 {
		return
	}
	for len(store.Items) > 1 {
		rendered := store.Render()
		if !TokenCountExceeds(rendered, budget) {
			return
		}
		trimmed := store.Items[0]
		store.Items = store.Items[1:]
		pruneSessionEvidenceFrozenItem(state, trimmed.ID)
	}
}

// GetVerificationTodo returns the raw serialized VerificationTodoStore JSON
// (no quoting). Suitable for DB persistence callers that want to manage their
// own quoting strategy.
func (s *SessionPromptState) GetVerificationTodo() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	return s.todoJSON
}

// SetVerificationTodo replaces the in-memory TODO state with the given JSON
// payload. Used during session restore from DB.
func (s *SessionPromptState) SetVerificationTodo(todoJSON string) {
	if s == nil {
		return
	}
	s.m.Lock()
	defer s.m.Unlock()
	s.todoJSON = todoJSON
}

// ApplyTodoDelta applies one normal ReAct action's optional todo_delta to the
// persisted TODO store, then re-serializes back to todoJSON. It returns one
// result entry per delta operation so callers can render a uniform summary;
// failures carry a non-empty Reason.
//
// 关键词: ApplyTodoDelta, 增量更新, DB 持久化, per-op 结果
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
// loop prompt injection. When currentScope is set, the snapshot groups items
// into CURRENT TASK vs OTHER TASKS sections. Empty string when no TODO has been
// tracked yet, so the prompt template can naturally skip the block.
func (s *SessionPromptState) GetVerificationTodoRendered(currentScope VerificationTodoScope) string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	if store.IsEmpty() {
		return ""
	}
	return store.RenderWithCurrentScope(currentScope)
}

// GetVerificationTodoMarkdownDelta returns the markdown snapshot computed
// against the current persisted state without mutating it. Callers should
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
