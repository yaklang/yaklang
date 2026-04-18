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
