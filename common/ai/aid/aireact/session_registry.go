package aireact

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

type eventSubscriber func(*schema.AiOutputEvent)

type runningSession struct {
	sessionID   string
	react       *ReAct
	subscribers []eventSubscriber
	mu          sync.RWMutex
}

var globalRunningSessions sync.Map // sessionID -> *runningSession

func registerRunningSession(sessionID string, react *ReAct) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || react == nil {
		return
	}
	rs := &runningSession{
		sessionID: sessionID,
		react:     react,
	}
	if _, loaded := globalRunningSessions.LoadOrStore(sessionID, rs); loaded {
		log.Warnf("aireact session %s already running, replacing registry entry", sessionID)
		globalRunningSessions.Store(sessionID, rs)
	}
}

func unregisterRunningSession(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	globalRunningSessions.Delete(sessionID)
}

// GetRunningSession returns a running ReAct instance by persistent session ID.
func GetRunningSession(sessionID string) (*ReAct, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false
	}
	v, ok := globalRunningSessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	rs, ok := v.(*runningSession)
	if !ok || rs.react == nil {
		return nil, false
	}
	return rs.react, true
}

// SubscribeRunningSession attaches an output event listener to a running session.
func SubscribeRunningSession(sessionID string, fn func(*schema.AiOutputEvent)) (func(), bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || fn == nil {
		return nil, false
	}
	v, ok := globalRunningSessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	rs, ok := v.(*runningSession)
	if !ok {
		return nil, false
	}
	return rs.subscribe(fn), true
}

func (rs *runningSession) subscribe(fn eventSubscriber) func() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.subscribers = append(rs.subscribers, fn)
	idx := len(rs.subscribers) - 1
	return func() {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		if idx >= 0 && idx < len(rs.subscribers) {
			rs.subscribers[idx] = nil
		}
	}
}

func (rs *runningSession) broadcast(event *schema.AiOutputEvent) {
	if rs == nil || event == nil {
		return
	}
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	for _, fn := range rs.subscribers {
		if fn != nil {
			fn(event)
		}
	}
}

func broadcastRunningSessionEvent(sessionID string, event *schema.AiOutputEvent) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || event == nil {
		return
	}
	v, ok := globalRunningSessions.Load(sessionID)
	if !ok {
		return
	}
	rs, ok := v.(*runningSession)
	if !ok {
		return
	}
	rs.broadcast(event)
}

func (r *ReAct) installRunningSessionRegistry() {
	sessionID := strings.TrimSpace(r.config.PersistentSessionId)
	if sessionID == "" {
		return
	}

	prevStart := r.config.EventLoopStartHook
	prevDone := r.config.EventLoopDoneHook
	prevHandler := r.config.EventHandler
	
	r.config.EventLoopStartHook = func() {
		if prevStart != nil {
			prevStart()
		}
		registerRunningSession(sessionID, r)
	}
	r.config.EventLoopDoneHook = func() {
		if prevDone != nil {
			prevDone()
		}
		unregisterRunningSession(sessionID)
	}
	r.config.EventHandler = func(e *schema.AiOutputEvent) {
		broadcastRunningSessionEvent(sessionID, e)
		if prevHandler != nil {
			prevHandler(e)
		}
	}
}
