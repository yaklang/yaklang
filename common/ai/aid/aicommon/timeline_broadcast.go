package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TimelineBroadcaster is shared by configs in one running session. Membership
// is keyed by runtime Timeline ID, independently of persistent session IDs.
// Its lock serializes delivery with fork snapshots and subscription changes.
type TimelineBroadcaster struct {
	mu          sync.Mutex
	subscribers map[string]*timelineSubscription
}

type timelineSubscription struct {
	timeline *Timeline
	refs     int
}

// All lease fields are protected by broadcaster.mu, including the cancellation
// hook: an already-cancelled context can invoke it before Subscribe returns.
type timelineBroadcastLease struct {
	broadcaster *TimelineBroadcaster
	timelineID  string
	released    bool
	stop        func() bool
}

func NewTimelineBroadcaster() *TimelineBroadcaster {
	return &TimelineBroadcaster{subscribers: make(map[string]*timelineSubscription)}
}

// GetTimelineID identifies this runtime container. It is never serialized or
// inherited by forks/read views; restoring a snapshot creates another ID.
func (m *Timeline) GetTimelineID() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timelineID == "" {
		m.timelineID = uuid.NewString()
	}
	return m.timelineID
}

func (m *Timeline) getTimelineBroadcaster() *TimelineBroadcaster {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.timelineBroadcaster
}

func (b *TimelineBroadcaster) subscribeLocked(timeline *Timeline, ctx context.Context) (*timelineBroadcastLease, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	timeline.mu.Lock()
	if timeline.timelineBroadcaster != nil && timeline.timelineBroadcaster != b {
		timeline.mu.Unlock()
		return nil, fmt.Errorf("timeline is already subscribed to another session")
	}
	if timeline.timelineID == "" {
		timeline.timelineID = uuid.NewString()
	}
	timeline.timelineBroadcaster = b
	id := timeline.timelineID
	timeline.mu.Unlock()
	if b.subscribers == nil {
		b.subscribers = make(map[string]*timelineSubscription)
	}
	entry := b.subscribers[id]
	if entry == nil {
		entry = &timelineSubscription{timeline: timeline}
		b.subscribers[id] = entry
	}
	entry.refs++
	lease := &timelineBroadcastLease{broadcaster: b, timelineID: id}
	if ctx != nil {
		lease.stop = context.AfterFunc(ctx, lease.release)
	}
	return lease, nil
}

func (l *timelineBroadcastLease) release() {
	if l == nil {
		return
	}
	b := l.broadcaster
	b.mu.Lock()
	defer b.mu.Unlock()
	if l.released {
		return
	}
	l.released = true
	if l.stop != nil {
		l.stop()
	}
	entry := b.subscribers[l.timelineID]
	entry.refs--
	if entry.refs == 0 {
		delete(b.subscribers, l.timelineID)
		entry.timeline.mu.Lock()
		entry.timeline.timelineBroadcaster = nil
		entry.timeline.mu.Unlock()
	}
}

// NewTimelineItem constructs a detached item suitable for explicit broadcast.
// The value must not be mutated concurrently with BroadcastTimelineItem.
func NewTimelineItem(value TimelineItemValue) *TimelineItem {
	return &TimelineItem{historyID: uuid.NewString(), createdAt: time.Now(), value: value}
}

// broadcast commits independent copies to every subscribed container before
// returning. Preflight under all recipient locks prevents partial ID conflicts.
// Observers and history writes run after all Timeline and broadcaster locks.
func (b *TimelineBroadcaster) broadcast(item *TimelineItem) (*TimelineItem, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, fmt.Errorf("serialize timeline broadcast: %w", err)
	}
	var canonical TimelineItem
	if err := json.Unmarshal(raw, &canonical); err != nil {
		return nil, err
	}
	id := canonical.GetID()
	if id <= 0 || canonical.deleted {
		return nil, fmt.Errorf("timeline broadcast requires a live item with a positive ID")
	}
	// Unknown value kinds otherwise silently round-trip as text with a lost ID.
	switch item.value.(type) {
	case *TextTimelineItem, *UserInteraction, *aitool.ToolResult, *PromotableTimelineItem:
	default:
		return nil, fmt.Errorf("unsupported timeline broadcast value %T", item.value)
	}

	b.mu.Lock()
	var notifications []func()
	var targets []*Timeline
	defer func() {
		for i := len(targets) - 1; i >= 0; i-- {
			targets[i].mu.Unlock()
		}
		b.mu.Unlock()
		for _, notify := range notifications {
			notify()
		}
	}()
	ids := make([]string, 0, len(b.subscribers))
	for timelineID := range b.subscribers {
		ids = append(ids, timelineID)
	}
	sort.Strings(ids)
	for _, timelineID := range ids {
		timeline := b.subscribers[timelineID].timeline
		timeline.mu.Lock()
		targets = append(targets, timeline)
	}
	copies := make([]*TimelineItem, len(targets))
	for i, timeline := range targets {
		if receipt, ok := timeline.broadcastReceipts[id]; ok {
			if receipt != canonical.historyID {
				return nil, fmt.Errorf("timeline broadcast: ID %d belongs to another item", id)
			}
			continue // Also deduplicates after compression, rollback or deletion.
		}
		if prior, ok := timeline.idToTimelineItem.Get(id); ok {
			if prior == nil || prior.historyID != canonical.historyID {
				return nil, fmt.Errorf("timeline broadcast: ID %d already exists in timeline %s", id, timeline.timelineID)
			}
			continue // The source may already contain this exact item.
		}
		var copy TimelineItem
		if err := json.Unmarshal(raw, &copy); err != nil {
			return nil, err
		}
		copies[i] = &copy
	}
	changed := false
	for i, timeline := range targets {
		if timeline.broadcastReceipts == nil {
			timeline.broadcastReceipts = make(map[int64]string)
		}
		timeline.broadcastReceipts[id] = canonical.historyID
		copy := copies[i]
		if copy == nil {
			continue
		}
		ts := copy.createdAt.UnixMilli()
		for timeline.tsToTimelineItem.Have(ts) {
			ts++
		}
		timeline.idToTs.Set(id, ts)
		timeline.insertTimelineItemLocked(ts, id, copy, &notifications)
		changed = true
	}
	if changed {
		return &canonical, nil
	}
	return nil, nil
}

func cloneTimelineBroadcastReceipts(receipts map[int64]string) map[int64]string {
	return maps.Clone(receipts)
}
