package lowhttp

import (
	"container/list"
	"context"
)

type h2DialCall struct {
	done   chan struct{}
	cancel context.CancelFunc
}

// Call only without a connection mutex held. Pool lock order is h2Mu -> conn.mu.
func (l *LowHttpConnPool) markH2Active(pc *persistConn) {
	l.h2Mu.Lock()
	if e := l.h2Idle[pc]; e != nil {
		l.h2IdleLRU.Remove(e)
		delete(l.h2Idle, pc)
	}
	l.h2Mu.Unlock()
}

func (l *LowHttpConnPool) markH2Idle(pc *persistConn) {
	if l.maxIdleConn <= 0 {
		return
	}
	var closeConns []*http2ClientConn
	l.h2Mu.Lock()
	c := pc.alt
	c.mu.Lock()
	idle := !c.closed && c.activeStreams == 0 && l.h2ConnMap[pc.cacheKey.hash()] == pc
	c.mu.Unlock()
	if idle {
		if l.h2Idle == nil {
			l.h2Idle = make(map[*persistConn]*list.Element)
			l.h2IdleLRU = list.New()
		}
		if e := l.h2Idle[pc]; e != nil {
			l.h2IdleLRU.MoveToBack(e)
		} else {
			l.h2Idle[pc] = l.h2IdleLRU.PushBack(pc)
		}
	}
	for len(l.h2Idle) > l.maxIdleConn {
		e := l.h2IdleLRU.Front()
		old := e.Value.(*persistConn)
		l.h2IdleLRU.Remove(e)
		delete(l.h2Idle, old)
		old.alt.mu.Lock()
		if old.alt.activeStreams == 0 && !old.alt.closed {
			// Reserve closure under the same lock used by newStream, so a request
			// cannot be accepted in the gap before the transport is actually closed.
			old.alt.closed = true
			closeConns = append(closeConns, old.alt)
		}
		old.alt.mu.Unlock()
	}
	l.h2Mu.Unlock()
	for _, c := range closeConns {
		c.setCloseReason("idle H2 connection limit")
		c.setClose()
	}
}
