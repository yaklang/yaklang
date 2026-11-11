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
	"github.com/yaklang/yaklang/common/log"
)

// TimelineBroadcaster fans one or more immutable Timeline items out to live
// timelines which belong to the same agent session.
type TimelineBroadcaster struct {
	mu              sync.RWMutex
	changed         *sync.Cond
	activePublishes int
	forkInProgress  bool
	registrations   map[string]func(...*TimelineItem) error
}

func NewTimelineBroadcaster() *TimelineBroadcaster {
	b := &TimelineBroadcaster{registrations: make(map[string]func(...*TimelineItem) error)}
	b.changed = sync.NewCond(&b.mu)
	return b
}

// Broadcast sends a group of items to the callbacks registered when Broadcast
// began. The whole group is validated before any callback is invoked.
// Callbacks run after the registration lock is released, so they may safely
// register, unregister or publish another item.
func (b *TimelineBroadcaster) Broadcast(items ...*TimelineItem) error {
	if b == nil {
		return fmt.Errorf("timeline broadcaster is nil")
	}
	canonical, err := normalizeTimelineBroadcastItems(items...)
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.waitForForkLocked()
	b.activePublishes++
	ids := make([]string, 0, len(b.registrations))
	callbacks := make(map[string]func(...*TimelineItem) error, len(b.registrations))
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
		if err := callbacks[id](canonical...); err != nil {
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
func (b *TimelineBroadcaster) Register(id string, callback func(...*TimelineItem) error) {
	if b == nil || id == "" || callback == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.registrations == nil {
		b.registrations = make(map[string]func(...*TimelineItem) error)
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

func normalizeTimelineBroadcastItems(items ...*TimelineItem) ([]*TimelineItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("timeline broadcast requires at least one item")
	}
	normalized := make([]*TimelineItem, 0, len(items))
	for index, item := range items {
		canonical, err := normalizeTimelineBroadcastItem(item)
		if err != nil {
			return nil, fmt.Errorf("timeline broadcast item %d: %w", index, err)
		}
		normalized = append(normalized, canonical)
	}
	return normalized, nil
}

// appendBroadcastItem 是注册到广播器的 Timeline 回调。
// 它先检查哪些 item 可以添加，再统一写入并记录广播回执。
func (m *Timeline) appendBroadcastItem(items ...*TimelineItem) error {
	if m == nil {
		return fmt.Errorf("timeline is required")
	}
	canonical, err := normalizeTimelineBroadcastItems(items...)
	if err != nil {
		return err
	}

	m.mu.Lock()
	var notifications []func()
	defer m.unlockAndNotifyTimeline(&notifications)

	checked := m.checkBroadcastItemsLocked(canonical)
	if err := m.commitBroadcastItemsLocked(checked, &notifications); err != nil {
		return err
	}
	return nil
}

// checkBroadcastItemsLocked 过滤已接收、批内重复和无意义的 evidence item。
// 调用方必须持有 Timeline 写锁。
func (m *Timeline) checkBroadcastItemsLocked(items []*TimelineItem) []*TimelineItem {
	checked := make([]*TimelineItem, 0, len(items))
	seenHistory := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, ok := m.broadcastReceipts[item.historyID]; ok {
			continue
		}
		if _, ok := seenHistory[item.historyID]; ok {
			continue
		}
		if promoted, ok := item.value.(*PromotableTimelineItem); ok &&
			promoted != nil && promoted.Kind == TimelinePromotedKindEvidence {
			evidenceOp, err := itemToEvidenceOp(item)
			if err != nil {
				log.Errorf("timeline broadcast skipped invalid evidence item %q: %v", item.historyID, err)
				continue
			}
			if !m.isEvidenceOpNeededLocked(evidenceOp) {
				continue
			}
		}
		seenHistory[item.historyID] = struct{}{}
		checked = append(checked, item)
	}
	return checked
}

// commitBroadcastItemsLocked 为检查通过的 item 分配本地 ID 并写入 Timeline。
// 待写入项会先全部完成 ID 分配，避免中途失败时批次只写入一部分。
func (m *Timeline) commitBroadcastItemsLocked(items []*TimelineItem, notifications *[]func()) error {
	if len(items) == 0 {
		return nil
	}
	config := m.config
	if config == nil {
		return fmt.Errorf("timeline has no config for broadcast ID allocation")
	}

	type preparedItem struct {
		item *TimelineItem
		id   int64
	}
	prepared := make([]preparedItem, 0, len(items))
	for _, item := range items {
		// ID provider 单调递增且保证唯一；出现碰撞说明 Timeline 状态异常。
		id := config.AcquireId()
		if id <= 0 {
			return fmt.Errorf("timeline ID provider returned non-positive ID %d", id)
		}
		if m.idToTimelineItem.Have(id) {
			log.Errorf("timeline broadcast skipped item %q: ID %d already exists", item.historyID, id)
			continue
		}
		item, err := timelineBroadcastItemWithID(item, id)
		if err != nil {
			return err
		}
		prepared = append(prepared, preparedItem{item: item, id: id})
	}

	if m.broadcastReceipts == nil {
		m.broadcastReceipts = make(map[string]struct{})
	}

	var lastTS int64
	for _, entry := range prepared {
		item := entry.item
		m.broadcastReceipts[item.historyID] = struct{}{}

		// 使用接收 Timeline 的当前时间，并保证批内时间戳严格递增。
		ts := time.Now().UnixMilli()
		for m.tsToTimelineItem.Have(ts) || ts <= lastTS {
			ts++
		}
		lastTS = ts
		m.idToTs.Set(entry.id, ts)
		m.invalidateFreezeFromLocked(entry.id)
		m.OrderInsertId(entry.id, item)
		m.OrderInsertTs(ts, item)
		m.collectItemInputCallbackLocked(notifications, item)
	}
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
