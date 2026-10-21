package aiknowledge

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

type buildProgress struct {
	name, lastEvent                        string
	started, changed                       time.Time
	analyzed, indexing, writing, persisted atomic.Int64
	mu                                     sync.Mutex // Serializes callbacks into the Yak VM as well as log output.
	closed                                 bool
	callback                               func(string)
	cancel                                 context.CancelFunc
	done                                   chan struct{}
}

func newBuildProgress(ctx context.Context, name string, callback func(string), interval time.Duration) *buildProgress {
	ctx, cancel := context.WithCancel(ctx)
	p := &buildProgress{name: name, started: time.Now(), changed: time.Now(), callback: callback, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.event("")
			}
		}
	}()
	return p
}

func (p *buildProgress) event(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if format != "" {
		p.lastEvent, p.changed = fmt.Sprintf(format, args...), time.Now()
	}
	message := fmt.Sprintf("knowledge build name=%q elapsed=%s analyzed_chunks=%d indexing_chunks=%d writing_chunks=%d persisted_entries=%d last_event=%q event_age=%s",
		p.name, time.Since(p.started).Round(time.Millisecond), p.analyzed.Load(), p.indexing.Load(), p.writing.Load(), p.persisted.Load(), p.lastEvent, time.Since(p.changed).Round(time.Millisecond))
	log.Info(message)
	if p.callback != nil {
		p.callback(message)
	}
}

func (p *buildProgress) finish(err error) {
	p.cancel()
	<-p.done
	if err != nil {
		p.event("failed; partial writes may remain: %v", err)
	} else {
		p.event("completed; all reported entries persisted")
	}
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
}
