package aicommon

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// TimelineItemInputEvent is an immutable receipt of a committed item write.
// ItemJSON uses TimelineItem's existing serialization, including exact user
// input and promotion payloads which ordinary prompt rendering can omit.
type TimelineItemInputEvent struct {
	ID        int64
	Timestamp time.Time
	ItemJSON  string
}

// TimelineCompressFreezeEvent describes the single atomic compression commit.
// Freeze includes only newly frozen items; Compression also covers previously
// frozen ordinary history retired by this transaction.
type TimelineCompressFreezeEvent struct {
	Freeze      TimelineFreezeResult
	Compression TimelineCompressionResult
}

// TimelineSummaryEvent is delivered after a successful compression commit.
// Summary is a completed, independent reader for each callback, not the live AI
// stream. Prompt is the rendered compression source; Instruction is its static
// instruction. Protocol wrapping/retry corrections are not included.
// RetainedIDs/RetainedRange select whole originals beside the committed summary.
type TimelineSummaryEvent struct {
	ThroughID     int64
	Summary       io.Reader
	Prompt        string
	Instruction   string
	RetainedIDs   []int64
	RetainedRange string
}

// TimelineMemoryEvent is delivered independently after the summary commit and
// full response. Err reports a failed memory tail; summary state is unchanged.
// Candidates are detached JSON, not persisted memory records.
type TimelineMemoryEvent struct {
	ThroughID      int64
	Prompt         string
	Instruction    string
	MemoryEntities []any
	Err            error
}

type timelineCallback[T any] struct {
	id string
	fn func(T)
}

type timelineCallbackRegistry[T any] struct {
	mu      sync.RWMutex
	entries []timelineCallback[T]
}

func (r *timelineCallbackRegistry[T]) register(id string, fn func(T)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, entry := range r.entries {
		if entry.id == id {
			if fn == nil {
				copy(r.entries[i:], r.entries[i+1:])
				r.entries[len(r.entries)-1] = timelineCallback[T]{}
				r.entries = r.entries[:len(r.entries)-1]
			} else {
				r.entries[i].fn = fn
			}
			return
		}
	}
	if fn != nil {
		r.entries = append(r.entries, timelineCallback[T]{id, fn})
	}
}

func (r *timelineCallbackRegistry[T]) snapshot() []timelineCallback[T] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]timelineCallback[T](nil), r.entries...)
}

func invokeTimelineCallback[T any](entry timelineCallback[T], event T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Warnf("timeline callback %q panicked: %v", entry.id, recovered)
		}
	}()
	entry.fn(event)
}

// Registration replaces the callback with the same ID in the same event kind.
// A nil callback unregisters that ID. Input/freeze/summary callbacks run
// synchronously after commit, outside Timeline.mu, in registration order.
// Memory completion is asynchronous. Callbacks should enqueue slow work.
// Concurrent writers can deliver callbacks concurrently. Panics are isolated.
// Registrations are runtime-only: they are not persisted, copied or forked.
func (m *Timeline) RegisterItemInputCallback(id string, fn func(TimelineItemInputEvent)) {
	m.itemInputCallbacks.register(id, fn)
}

func (m *Timeline) RegisterFreezeCallback(id string, fn func(TimelineFreezeResult)) {
	m.freezeCallbacks.register(id, fn)
}

func (m *Timeline) RegisterCompressFreezeCallback(id string, fn func(TimelineCompressFreezeEvent)) {
	m.compressFreezeCallbacks.register(id, fn)
}

func (m *Timeline) RegisterSummaryCallback(id string, fn func(TimelineSummaryEvent)) {
	m.summaryCallbacks.register(id, fn)
}

// Memory delivery runs separately so storage or slow consumers cannot block
// the summary commit or the next loop. Replacing/unregistering uses the same ID.
func (m *Timeline) RegisterMemoryCallback(id string, fn func(TimelineMemoryEvent)) {
	m.memoryCallbacks.register(id, fn)
}

// Capture under Timeline.mu; invoke only after the complete write is unlocked.
// No serialization or event allocation is needed without input listeners.
func (m *Timeline) collectItemInputCallbackLocked(notifications *[]func(), item *TimelineItem) {
	if notifications == nil {
		return
	}
	callbacks := m.itemInputCallbacks.snapshot()
	if len(callbacks) == 0 {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Warnf("snapshot timeline callback item panicked: %v", recovered)
		}
	}()
	raw, err := json.Marshal(item)
	if err != nil {
		log.Warnf("snapshot timeline callback item: %v", err)
		return
	}
	event := TimelineItemInputEvent{ID: item.GetID(), Timestamp: item.createdAt, ItemJSON: string(raw)}
	*notifications = append(*notifications, func() {
		for _, callback := range callbacks {
			invokeTimelineCallback(callback, event)
		}
	})
}

func (m *Timeline) unlockAndNotifyTimeline(notifications *[]func()) {
	m.mu.Unlock()
	for _, notify := range *notifications {
		notify()
	}
}

func cloneTimelineFreezeResult(result TimelineFreezeResult) TimelineFreezeResult {
	result.NewlyFrozenIDs = append([]int64(nil), result.NewlyFrozenIDs...)
	result.Promotions = append([]PromotableTimelineItem(nil), result.Promotions...)
	return result
}

func (m *Timeline) notifyFreeze(result TimelineFreezeResult) {
	if len(result.NewlyFrozenIDs) == 0 {
		return
	}
	for _, callback := range m.freezeCallbacks.snapshot() {
		invokeTimelineCallback(callback, cloneTimelineFreezeResult(result))
	}
}

func (m *Timeline) compressionCallbackLocked(snapshot *timelineCompressionSnapshot, result *TimelineCompressionResult) func() {
	// Capture all lists before invoking user code. Registration changes made by
	// one callback apply to later transactions, not later stages of this commit.
	freezeCallbacks := m.freezeCallbacks.snapshot()
	compressCallbacks := m.compressFreezeCallbacks.snapshot()
	summaryCallbacks := m.summaryCallbacks.snapshot()
	memoryCallbacks := m.memoryCallbacks.snapshot()
	if len(freezeCallbacks)+len(compressCallbacks)+len(summaryCallbacks)+len(memoryCallbacks) == 0 {
		return nil
	}
	freeze := TimelineFreezeResult{Version: snapshot.FreezeVersion + 1, ThroughID: snapshot.ThroughID}
	if len(freezeCallbacks)+len(compressCallbacks) > 0 {
		for _, id := range m.idToTimelineItem.Keys() {
			if id <= snapshot.FrozenThroughID || id > snapshot.ThroughID {
				continue
			}
			if item, ok := m.idToTimelineItem.Get(id); ok && item != nil && !item.deleted {
				freeze.NewlyFrozenIDs = append(freeze.NewlyFrozenIDs, id)
				if control := timelinePromotionForItem(item); control != nil {
					freeze.Promotions = append(freeze.Promotions, *control)
				}
			}
		}
	}
	snapshot.CommittedFreeze = freeze
	return func() {
		freeze := snapshot.CommittedFreeze
		if len(freeze.NewlyFrozenIDs) > 0 {
			for _, callback := range freezeCallbacks {
				invokeTimelineCallback(callback, cloneTimelineFreezeResult(freeze))
			}
		}
		for _, callback := range compressCallbacks {
			compression := *result
			compression.RetiredIDs = append([]int64(nil), result.RetiredIDs...)
			compression.RetainedIDs = append([]int64(nil), result.RetainedIDs...)
			invokeTimelineCallback(callback, TimelineCompressFreezeEvent{cloneTimelineFreezeResult(freeze), compression})
		}
		// Exact-only commits promote journals without asking AI for a summary.
		if snapshot.SummaryPrompt == "" {
			return
		}
		for _, callback := range summaryCallbacks {
			invokeTimelineCallback(callback, TimelineSummaryEvent{
				ThroughID: result.ThroughID, Summary: strings.NewReader(result.Summary),
				Prompt: snapshot.SummaryPrompt, Instruction: timelineCompressionInstruction,
				RetainedIDs: append([]int64(nil), result.RetainedIDs...), RetainedRange: result.RetainedRange,
			})
		}
		if len(memoryCallbacks) == 0 || snapshot.MemoryCompletion == nil {
			return
		}
		go func() {
			completion := snapshot.MemoryCompletion
			<-completion.done
			var entities []any
			if completion.err == nil && completion.output != nil {
				entities = completion.output.MemoryEntities
			}
			for _, callback := range memoryCallbacks {
				invokeTimelineCallback(callback, TimelineMemoryEvent{ThroughID: result.ThroughID,
					Prompt: snapshot.SummaryPrompt, Instruction: timelineCompressionInstruction,
					MemoryEntities: cloneCompressionMemoryEntities(entities), Err: completion.err})
			}
		}()
	}
}
