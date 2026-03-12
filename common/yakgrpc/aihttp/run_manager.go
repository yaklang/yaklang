package aihttp

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type RunSession struct {
	RunID              string
	Status             RunStatus
	StartParams        *ypb.AIStartParams
	StartAttachedFiles []string

	subscribers   map[string]chan RunEvent
	subscribersMu sync.RWMutex

	inputChan *chanx.UnlimitedChan[*ypb.AIInputEvent]

	ctx    context.Context
	cancel context.CancelFunc

	streamStartMu sync.Mutex
	streamStarted bool

	CreatedAt  time.Time
	FinishedAt *time.Time
	Error      string
}

func NewRunSession(parentCtx context.Context, runID string, startParams *ypb.AIStartParams, startAttachedFiles []string) *RunSession {
	ctx, cancel := context.WithCancel(parentCtx)
	if runID == "" {
		runID = uuid.New().String()
	}
	return &RunSession{
		RunID:              runID,
		Status:             RunStatusPending,
		StartParams:        startParams,
		StartAttachedFiles: append([]string(nil), startAttachedFiles...),
		subscribers:        make(map[string]chan RunEvent),
		inputChan:          chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10),
		ctx:                ctx,
		cancel:             cancel,
		CreatedAt:          time.Now(),
	}
}

func (rs *RunSession) AddEvent(e RunEvent) {
	rs.subscribersMu.RLock()
	defer rs.subscribersMu.RUnlock()
	for _, ch := range rs.subscribers {
		select {
		case ch <- e:
			// default:
			// 	log.Debugf("subscriber channel full, dropping event %s", e.ID)
		}
	}
}

func (rs *RunSession) Subscribe() (string, chan RunEvent) {
	id := uuid.New().String()
	ch := make(chan RunEvent, 256)

	rs.subscribersMu.Lock()
	rs.subscribers[id] = ch
	rs.subscribersMu.Unlock()

	return id, ch
}

func (rs *RunSession) Unsubscribe(id string) {
	rs.subscribersMu.Lock()
	if ch, ok := rs.subscribers[id]; ok {
		close(ch)
		delete(rs.subscribers, id)
	}
	rs.subscribersMu.Unlock()
}

func (rs *RunSession) PushInput(event *ypb.AIInputEvent) {
	rs.inputChan.SafeFeed(event)
}

func (rs *RunSession) MarkStreamStarted() bool {
	rs.streamStartMu.Lock()
	defer rs.streamStartMu.Unlock()
	if rs.streamStarted {
		return false
	}
	rs.streamStarted = true
	return true
}

func (rs *RunSession) Complete(err error) {
	if rs.Status == RunStatusCancelled {
		return
	}

	now := time.Now()
	rs.FinishedAt = &now
	if err != nil {
		rs.Status = RunStatusFailed
		rs.Error = err.Error()
	} else {
		rs.Status = RunStatusCompleted
	}

	doneEvent := RunEvent{
		ID:        uuid.New().String(),
		Type:      "done",
		Timestamp: now.Unix(),
	}
	if err != nil {
		doneEvent.Type = "error"
		doneBytes, _ := json.Marshal(map[string]string{"error": err.Error()})
		doneEvent.Content = string(doneBytes)
	}
	rs.AddEvent(doneEvent)
}

func (rs *RunSession) Cancel() {
	if rs.Status == RunStatusCompleted || rs.Status == RunStatusFailed || rs.Status == RunStatusCancelled {
		return
	}
	rs.Status = RunStatusCancelled
	now := time.Now()
	rs.FinishedAt = &now

	doneEvent := RunEvent{
		ID:        uuid.New().String(),
		Type:      "done",
		Timestamp: now.Unix(),
	}
	doneBytes, _ := json.Marshal(map[string]string{"status": "cancelled"})
	doneEvent.Content = string(doneBytes)
	rs.AddEvent(doneEvent)

	rs.cancel()
}

type RunManager struct {
	sessions map[string]*RunSession
	mu       sync.RWMutex
	ctx      context.Context
}

func NewRunManager(ctx context.Context) *RunManager {
	return &RunManager{
		sessions: make(map[string]*RunSession),
		ctx:      ctx,
	}
}

func (rm *RunManager) Create(runID string, startParams *ypb.AIStartParams, startAttachedFiles []string) *RunSession {
	session := NewRunSession(rm.ctx, runID, startParams, startAttachedFiles)

	rm.mu.Lock()
	rm.sessions[session.RunID] = session
	rm.mu.Unlock()

	return session
}

func (rm *RunManager) Get(runID string) (*RunSession, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	s, ok := rm.sessions[runID]
	return s, ok
}

func (rm *RunManager) Remove(runID string) {
	rm.mu.Lock()
	delete(rm.sessions, runID)
	rm.mu.Unlock()
}

func (rm *RunManager) ListActive() []SessionItem {
	return rm.listSessions(func(s *RunSession) bool {
		return s.Status == RunStatusPending || s.Status == RunStatusRunning
	})
}

func (rm *RunManager) ListAll() []SessionItem {
	return rm.listSessions(nil)
}

func (rm *RunManager) listSessions(filter func(*RunSession) bool) []SessionItem {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	items := make([]SessionItem, 0, len(rm.sessions))
	for _, s := range rm.sessions {
		if filter != nil && !filter(s) {
			continue
		}
		items = append(items, SessionItem{
			RunID:     s.RunID,
			Status:    s.Status,
			CreatedAt: s.CreatedAt,
			IsAlive:   s.Status == RunStatusPending || s.Status == RunStatusRunning,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

func (rm *RunManager) CancelAll() {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	for _, s := range rm.sessions {
		if s.Status == RunStatusRunning || s.Status == RunStatusPending {
			s.Cancel()
		}
	}
}
