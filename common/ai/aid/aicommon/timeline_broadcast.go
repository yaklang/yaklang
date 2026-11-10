package aicommon

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TimelineBroadcaster fans one immutable Timeline item out to live timelines
// which belong to the same agent session.
type TimelineBroadcaster struct {
	mu          sync.RWMutex
	nextID      uint64
	subscribers map[uint64]func(*TimelineItem) error
}

func NewTimelineBroadcaster() *TimelineBroadcaster {
	return &TimelineBroadcaster{subscribers: make(map[uint64]func(*TimelineItem) error)}
}

// Broadcast sends item to the subscribers which existed when Broadcast began.
// Subscribers run after the registry lock is released, so a callback may
// safely subscribe, unsubscribe or publish another item.
func (b *TimelineBroadcaster) Broadcast(item *TimelineItem) error {
	if b == nil {
		return fmt.Errorf("timeline broadcaster is nil")
	}
	canonical, err := normalizeTimelineBroadcastItem(item)
	if err != nil {
		return err
	}

	b.mu.RLock()
	ids := make([]uint64, 0, len(b.subscribers))
	callbacks := make(map[uint64]func(*TimelineItem) error, len(b.subscribers))
	for id, callback := range b.subscribers {
		ids = append(ids, id)
		callbacks[id] = callback
	}
	b.mu.RUnlock()

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var failures []error
	for _, id := range ids {
		if err := callbacks[id](canonical); err != nil {
			failures = append(failures, fmt.Errorf("timeline subscriber %d: %w", id, err))
		}
	}
	return errors.Join(failures...)
}

// Subscribe registers a callback and returns the token used by Unsubscribe.
func (b *TimelineBroadcaster) Subscribe(callback func(*TimelineItem) error) uint64 {
	if b == nil || callback == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribers == nil {
		b.subscribers = make(map[uint64]func(*TimelineItem) error)
	}
	b.nextID++
	id := b.nextID
	b.subscribers[id] = callback
	return id
}

// Unsubscribe removes one callback. Unknown or zero tokens are ignored.
func (b *TimelineBroadcaster) Unsubscribe(id uint64) {
	if b == nil || id == 0 {
		return
	}
	b.mu.Lock()
	delete(b.subscribers, id)
	b.mu.Unlock()
}

// NewTimelineItem creates a detached item for an explicit session broadcast.
func NewTimelineItem(value TimelineItemValue) *TimelineItem {
	return &TimelineItem{historyID: uuid.NewString(), createdAt: time.Now(), value: value}
}

func normalizeTimelineBroadcastItem(item *TimelineItem) (*TimelineItem, error) {
	if item == nil || item.value == nil {
		return nil, fmt.Errorf("timeline broadcast item is required")
	}
	switch item.value.(type) {
	case *TextTimelineItem, *UserInteraction, *aitool.ToolResult, *PromotableTimelineItem:
	default:
		return nil, fmt.Errorf("unsupported timeline broadcast item %T", item.value)
	}
	if item.GetID() <= 0 || item.deleted {
		return nil, fmt.Errorf("timeline broadcast requires a live item with a positive ID")
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, fmt.Errorf("serialize timeline broadcast item: %w", err)
	}
	var canonical TimelineItem
	if err := json.Unmarshal(raw, &canonical); err != nil {
		return nil, fmt.Errorf("copy timeline broadcast item: %w", err)
	}
	return &canonical, nil
}

// appendBroadcastItem is the Timeline-side subscriber. Each timeline owns its
// copy, validates the item and commits it through the normal index path.
func (m *Timeline) appendBroadcastItem(item *TimelineItem) error {
	if m == nil || item == nil || item.value == nil {
		return fmt.Errorf("timeline and item are required")
	}
	copy, err := normalizeTimelineBroadcastItem(item)
	if err != nil {
		return err
	}
	copy.broadcast = true

	m.mu.Lock()
	var notifications []func()
	defer m.unlockAndNotifyTimeline(&notifications)

	id := copy.GetID()
	if existing, ok := m.idToTimelineItem.Get(id); ok {
		if existing == nil || existing.historyID != copy.historyID {
			return fmt.Errorf("timeline item ID %d already belongs to another value", id)
		}
		return nil
	}

	ts := copy.createdAt.UnixMilli()
	if ts <= 0 {
		ts = time.Now().UnixMilli()
	}
	for m.tsToTimelineItem.Have(ts) {
		ts++
	}
	m.idToTs.Set(id, ts)
	m.invalidateFreezeFromLocked(id)
	m.OrderInsertId(id, copy)
	m.OrderInsertTs(ts, copy)
	m.collectItemInputCallbackLocked(&notifications, copy)
	return nil
}
