package aicommon

import (
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
	// TODO list maintained by VerifyUserSatisfaction. The list is rendered
	// into every loop prompt (timeline-open section, right after
	// SessionEvidence) so the model can see its own pending TODOs on every
	// iteration, not only at Verify checkpoints.
	//
	// 关键词: todoJSON, VerificationTodoStore 序列化, SessionEvidence 同构,
	//        全局 TODO 持久态
	todoJSON string
}

func NewSessionPromptState() *SessionPromptState {
	return &SessionPromptState{}
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
	store.ShrinkToTokenBudget(sessionEvidenceTokenBudget)
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

// ApplyVerificationTodoOps applies one verification round's next_movements
// operations (and the round's satisfied flag) to the persisted TODO store,
// then re-serializes back to todoJSON. Returns the quoted serialized JSON so
// callers can persist it (mirroring ApplySessionEvidenceOps).
//
// 关键词: ApplyVerificationTodoOps, 增量更新, satisfied -> SKIPPED, DB 持久化
func (s *SessionPromptState) ApplyVerificationTodoOps(satisfied bool, movements []VerifyNextMovement) string {
	if s == nil {
		return ""
	}
	s.m.Lock()
	defer s.m.Unlock()

	store := UnmarshalVerificationTodoStore(s.todoJSON)
	store.Apply(satisfied, movements)
	s.todoJSON = store.Marshal()
	return codec.StrConvQuote(s.todoJSON)
}

// GetVerificationTodoRendered returns the plain-text TODO snapshot ready for
// loop prompt injection. Empty string when no TODO has been tracked yet, so
// the prompt template can naturally skip the block.
func (s *SessionPromptState) GetVerificationTodoRendered() string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	if store.IsEmpty() {
		return ""
	}
	return store.Render()
}

// GetVerificationTodoMarkdownDelta returns the markdown snapshot computed
// against the current persisted state without mutating it. Callers should
// invoke this BEFORE ApplyVerificationTodoOps so the (new) / (done) markers
// are derived from the pre-apply state.
//
// 关键词: GetVerificationTodoMarkdownDelta, 预览模式, 不变更状态
func (s *SessionPromptState) GetVerificationTodoMarkdownDelta(satisfied bool, movements []VerifyNextMovement) string {
	if s == nil {
		return ""
	}
	s.m.RLock()
	defer s.m.RUnlock()
	store := UnmarshalVerificationTodoStore(s.todoJSON)
	return store.RenderMarkdownDelta(satisfied, movements)
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
