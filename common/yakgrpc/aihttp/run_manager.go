package aihttp

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// RunSession represents an active AI run session
type RunSession struct {
	// RunID is the unique identifier for this run
	RunID string
	// TaskID is the client-provided task identifier
	TaskID string
	// CoordinatorID is the AI coordinator ID
	CoordinatorID string
	// Status is the current run status
	Status RunStatus
	// StartTime is when the run started
	StartTime time.Time
	// EndTime is when the run ended
	EndTime *time.Time
	// Error contains any error message
	Error string

	// Context and cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Event channels
	inputChan  *chanx.UnlimitedChan[*ypb.AIInputEvent]
	outputChan chan *ypb.AIOutputEvent

	// Events buffer for replay
	events      []*ypb.AIOutputEvent
	eventsMutex sync.RWMutex

	// Subscribers for SSE
	subscribers      map[string]chan *ypb.AIOutputEvent
	subscribersMutex sync.RWMutex
}

// RunManager manages all active run sessions
type RunManager struct {
	sessions sync.Map // map[string]*RunSession
}

// NewRunManager creates a new RunManager
func NewRunManager() *RunManager {
	return &RunManager{}
}

// CreateSession creates a new run session
func (rm *RunManager) CreateSession(ctx context.Context, taskID string) *RunSession {
	runID := uuid.New().String()
	sessionCtx, cancel := context.WithCancel(ctx)

	session := &RunSession{
		RunID:       runID,
		TaskID:      taskID,
		Status:      RunStatusPending,
		StartTime:   time.Now(),
		ctx:         sessionCtx,
		cancel:      cancel,
		inputChan:   chanx.NewUnlimitedChan[*ypb.AIInputEvent](sessionCtx, 10),
		outputChan:  make(chan *ypb.AIOutputEvent, 100),
		events:      make([]*ypb.AIOutputEvent, 0),
		subscribers: make(map[string]chan *ypb.AIOutputEvent),
	}

	rm.sessions.Store(runID, session)
	log.Infof("Created new run session: runID=%s, taskID=%s", runID, taskID)

	return session
}

// GetSession retrieves a session by run ID
func (rm *RunManager) GetSession(runID string) (*RunSession, bool) {
	if v, ok := rm.sessions.Load(runID); ok {
		return v.(*RunSession), true
	}
	return nil, false
}

// DeleteSession removes a session
func (rm *RunManager) DeleteSession(runID string) {
	if session, ok := rm.GetSession(runID); ok {
		session.cancel()
		rm.sessions.Delete(runID)
		log.Infof("Deleted run session: runID=%s", runID)
	}
}

// CancelSession cancels a running session
func (rm *RunManager) CancelSession(runID string) bool {
	session, ok := rm.GetSession(runID)
	if !ok {
		return false
	}

	session.cancel()
	session.Status = RunStatusCancelled
	now := time.Now()
	session.EndTime = &now
	log.Infof("Cancelled run session: runID=%s", runID)

	return true
}

// CancelAll cancels all active sessions
func (rm *RunManager) CancelAll() {
	rm.sessions.Range(func(key, value interface{}) bool {
		if session, ok := value.(*RunSession); ok {
			session.cancel()
			session.Status = RunStatusCancelled
		}
		return true
	})
	log.Info("Cancelled all run sessions")
}

// ListSessions returns all active sessions
func (rm *RunManager) ListSessions() []*RunSession {
	var sessions []*RunSession
	rm.sessions.Range(func(key, value interface{}) bool {
		if session, ok := value.(*RunSession); ok {
			sessions = append(sessions, session)
		}
		return true
	})
	return sessions
}

// --- RunSession methods ---

// SetStatus updates the session status
func (s *RunSession) SetStatus(status RunStatus) {
	s.Status = status
	if status == RunStatusCompleted || status == RunStatusFailed || status == RunStatusCancelled {
		now := time.Now()
		s.EndTime = &now
	}
}

// SetCoordinatorID sets the coordinator ID
func (s *RunSession) SetCoordinatorID(id string) {
	s.CoordinatorID = id
}

// SetError sets an error message
func (s *RunSession) SetError(err string) {
	s.Error = err
}

// Context returns the session context
func (s *RunSession) Context() context.Context {
	return s.ctx
}

// Cancel cancels the session
func (s *RunSession) Cancel() {
	s.cancel()
}

// GetInputChan returns the input channel
func (s *RunSession) GetInputChan() *chanx.UnlimitedChan[*ypb.AIInputEvent] {
	return s.inputChan
}

// SendInput sends an input event to the session
func (s *RunSession) SendInput(event *ypb.AIInputEvent) {
	s.inputChan.SafeFeed(event)
}

// AddEvent adds an output event to the session
func (s *RunSession) AddEvent(event *ypb.AIOutputEvent) {
	s.eventsMutex.Lock()
	s.events = append(s.events, event)
	s.eventsMutex.Unlock()

	// Notify all subscribers
	s.subscribersMutex.RLock()
	for _, ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			// Channel full, skip
		}
	}
	s.subscribersMutex.RUnlock()
}

// GetEvents returns all events
func (s *RunSession) GetEvents() []*ypb.AIOutputEvent {
	s.eventsMutex.RLock()
	defer s.eventsMutex.RUnlock()
	result := make([]*ypb.AIOutputEvent, len(s.events))
	copy(result, s.events)
	return result
}

// GetEventsSince returns events since the given timestamp
func (s *RunSession) GetEventsSince(since int64) []*ypb.AIOutputEvent {
	s.eventsMutex.RLock()
	defer s.eventsMutex.RUnlock()

	var result []*ypb.AIOutputEvent
	for _, event := range s.events {
		if event.Timestamp >= since {
			result = append(result, event)
		}
	}
	return result
}

// Subscribe creates a new subscriber for this session
func (s *RunSession) Subscribe(subscriberID string) chan *ypb.AIOutputEvent {
	ch := make(chan *ypb.AIOutputEvent, 100)
	s.subscribersMutex.Lock()
	s.subscribers[subscriberID] = ch
	s.subscribersMutex.Unlock()
	return ch
}

// Unsubscribe removes a subscriber
func (s *RunSession) Unsubscribe(subscriberID string) {
	s.subscribersMutex.Lock()
	if ch, ok := s.subscribers[subscriberID]; ok {
		close(ch)
		delete(s.subscribers, subscriberID)
	}
	s.subscribersMutex.Unlock()
}

// IsDone returns true if the session is done
func (s *RunSession) IsDone() bool {
	return s.Status == RunStatusCompleted ||
		s.Status == RunStatusFailed ||
		s.Status == RunStatusCancelled
}
