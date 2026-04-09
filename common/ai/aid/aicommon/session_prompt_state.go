package aicommon

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

// SessionPromptState holds session-scoped prompt rendering data that must stay
// consistent across configs sharing the same conversation.
type SessionPromptState struct {
	m sync.RWMutex

	UserInputHistory []schema.AIAgentUserInputRecord
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
