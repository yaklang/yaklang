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
	mu              sync.RWMutex
	changed         *sync.Cond
	activePublishes int
	forkInProgress  bool
	registrations   map[string]func(*TimelineItem) error
}

func NewTimelineBroadcaster() *TimelineBroadcaster {
	b := &TimelineBroadcaster{registrations: make(map[string]func(*TimelineItem) error)}
	b.changed = sync.NewCond(&b.mu)
	return b
}

// Broadcast sends item to the callbacks registered when Broadcast began.
// Callbacks run after the registration lock is released, so they may safely
// register, unregister or publish another item.
func (b *TimelineBroadcaster) Broadcast(item *TimelineItem) error {
	if b == nil {
		return fmt.Errorf("timeline broadcaster is nil")
	}
	canonical, err := normalizeTimelineBroadcastItem(item)
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.waitForForkLocked()
	b.activePublishes++
	ids := make([]string, 0, len(b.registrations))
	callbacks := make(map[string]func(*TimelineItem) error, len(b.registrations))
	for id, callback := range b.registrations {
		ids = append(ids, id)
		callbacks[id] = callback
	}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.activePublishes--
		b.changed.Broadcast()
		b.mu.Unlock()
	}()

	sort.Strings(ids)
	var failures []error
	for _, id := range ids {
		if err := callbacks[id](canonical); err != nil {
			failures = append(failures, fmt.Errorf("timeline registration %q: %w", id, err))
		}
	}
	return errors.Join(failures...)
}

func (b *TimelineBroadcaster) waitForForkLocked() {
	if b.changed == nil {
		b.changed = sync.NewCond(&b.mu)
	}
	for b.forkInProgress {
		b.changed.Wait()
	}
}

// beginFork prevents a publication from landing between the parent snapshot
// and child registration. Publications which already started finish first.
func (b *TimelineBroadcaster) beginFork() func() {
	if b == nil {
		return func() {}
	}
	b.mu.Lock()
	if b.changed == nil {
		b.changed = sync.NewCond(&b.mu)
	}
	for b.forkInProgress || b.activePublishes > 0 {
		b.changed.Wait()
	}
	b.forkInProgress = true
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.forkInProgress = false
			b.changed.Broadcast()
			b.mu.Unlock()
		})
	}
}

// Register adds or replaces one stable callback registration.
func (b *TimelineBroadcaster) Register(id string, callback func(*TimelineItem) error) {
	if b == nil || id == "" || callback == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.registrations == nil {
		b.registrations = make(map[string]func(*TimelineItem) error)
	}
	b.registrations[id] = callback
}

// Unregister removes one callback. Unknown or empty keys are ignored.
func (b *TimelineBroadcaster) Unregister(id string) {
	if b == nil || id == "" {
		return
	}
	b.mu.Lock()
	delete(b.registrations, id)
	b.mu.Unlock()
}

// GetTimelineID identifies this runtime Timeline container. The ID is local to
// this process and is not persisted or inherited by fork snapshots/read views.
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
	if item.GetID() != 0 {
		return nil, fmt.Errorf("timeline broadcast item ID must be empty, got %d", item.GetID())
	}
	if item.deleted {
		return nil, fmt.Errorf("timeline broadcast item must be live")
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

// appendBroadcastItem is a registered Timeline callback. Each Timeline owns
// its copy, validates the item and commits it through the normal index path.
func (m *Timeline) appendBroadcastItem(item *TimelineItem) error {
	if m == nil || item == nil || item.value == nil {
		return fmt.Errorf("timeline and item are required")
	}
	copy, err := normalizeTimelineBroadcastItem(item)
	if err != nil {
		return err
	}

	m.mu.RLock()
	_, alreadyCommitted := m.broadcastReceipts[copy.historyID]
	config := m.config
	m.mu.RUnlock()
	if alreadyCommitted {
		return nil
	}
	if config == nil {
		return fmt.Errorf("timeline has no config for broadcast ID allocation")
	}

	m.mu.Lock()
	var notifications []func()
	defer m.unlockAndNotifyTimeline(&notifications)

	if _, ok := m.broadcastReceipts[copy.historyID]; ok {
		return nil
	}
	var id int64
	for {
		id = config.AcquireId()
		if id <= 0 {
			return fmt.Errorf("timeline ID provider returned non-positive ID %d", id)
		}
		if !m.idToTimelineItem.Have(id) {
			break
		}
	}
	copy, err = timelineBroadcastItemWithID(copy, id)
	if err != nil {
		return err
	}
	if m.broadcastReceipts == nil {
		m.broadcastReceipts = make(map[string]struct{})
	}
	m.broadcastReceipts[copy.historyID] = struct{}{}

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

func timelineBroadcastItemWithID(item *TimelineItem, id int64) (*TimelineItem, error) {
	if item == nil || item.value == nil || id <= 0 {
		return nil, fmt.Errorf("timeline broadcast item and positive ID are required")
	}
	copy := *item
	switch value := item.value.(type) {
	case *TextTimelineItem:
		cloned := *value
		cloned.ID = id
		copy.value = &cloned
	case *UserInteraction:
		cloned := *value
		cloned.ID = id
		copy.value = &cloned
	case *aitool.ToolResult:
		cloned := *value
		cloned.ID = id
		copy.value = &cloned
	case *PromotableTimelineItem:
		cloned := *value
		cloned.ID = id
		copy.value = &cloned
	default:
		return nil, fmt.Errorf("unsupported timeline broadcast item %T", item.value)
	}
	return &copy, nil
}
