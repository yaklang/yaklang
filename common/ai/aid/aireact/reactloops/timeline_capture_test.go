package reactloops

import (
	"context"
	"sync"

	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

// Independent stall/sub-agent tests share a synchronized timeline recorder.
type timelineCapturingInvoker struct {
	*mockcfg.MockInvoker
	mu      sync.Mutex
	entries []timelineEntry
}

type timelineEntry struct {
	Tag     string
	Content string
}

func newTimelineCapturingInvoker(ctx context.Context) *timelineCapturingInvoker {
	return &timelineCapturingInvoker{MockInvoker: mockcfg.NewMockInvoker(ctx)}
}

func (i *timelineCapturingInvoker) AddToTimeline(tag, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.entries = append(i.entries, timelineEntry{Tag: tag, Content: content})
}

func (i *timelineCapturingInvoker) Entries() []timelineEntry {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]timelineEntry(nil), i.entries...)
}

func (i *timelineCapturingInvoker) hasTag(tag string) bool {
	for _, entry := range i.Entries() {
		if entry.Tag == tag {
			return true
		}
	}
	return false
}
