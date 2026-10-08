package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Session candidates are extraction history, not verified or persisted memory
// entities. Forks share this append-only ledger without sharing ordinary history.
// Only successful, committed compression responses enter it. Pending streams are
// runtime-only; snapshots persist completed candidates and tolerate older dumps.
type timelineSessionMemory struct {
	mu                sync.Mutex
	callbacks         timelineCallbackRegistry[TimelineMemoryEvent]
	owner             *Timeline
	items             []any
	seen              map[string]bool
	pending           []*timelineCompressionCompletion
	retries           map[string]timelineMemoryRetry
	processed         map[string]bool // durable source receipts, never prompt data
	persistenceConfig AICallerConfigIf
	work              chan struct{} // serialize finalization/recovery across session forks
}

// IsSessionTimeline identifies the owner of the shared extraction history.
// Persistence is attached there, rather than rebound to a task's shorter lifetime.
func (m *Timeline) IsSessionTimeline() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.branchTimeline && m.sessionMemory != nil && m.sessionMemory.owner == m
}

func newTimelineSessionMemory(owner *Timeline, items []any) *timelineSessionMemory {
	h := &timelineSessionMemory{owner: owner, seen: make(map[string]bool), retries: make(map[string]timelineMemoryRetry), processed: make(map[string]bool), work: make(chan struct{}, 1)}
	h.appendLocked(cloneCompressionMemoryEntities(items))
	return h
}

func (h *timelineSessionMemory) appendLocked(items []any) bool {
	changed := false
	for _, item := range items {
		if !validCompressionMemoryEntity(item) {
			continue
		}
		raw, _ := json.Marshal(item)
		key := string(raw)
		// Deduplicate exact JSON only. Keep corrections and distinct claims; an
		// extraction history must not silently perform semantic memory merging.
		if !h.seen[key] {
			h.seen[key] = true
			h.items = append(h.items, item)
			changed = true
		}
	}
	return changed
}

func (h *timelineSessionMemory) collectLocked() bool {
	changed := false
	remaining := h.pending[:0]
	for _, completion := range h.pending {
		select {
		case <-completion.done:
			if completion.err == nil && completion.output != nil {
				changed = h.appendLocked(cloneCompressionMemoryEntities(completion.output.MemoryEntities)) || changed
				if len(completion.output.MemoryEntities) == 0 {
					delete(h.retries, completion.retryKey)
				}
				h.processed[completion.retryKey] = true
			}
			if completion.notified != nil {
				select {
				case <-completion.notified:
				default:
					remaining = append(remaining, completion)
				}
			}
		default:
			remaining = append(remaining, completion)
		}
	}
	clear(h.pending[len(remaining):])
	h.pending = remaining
	return changed
}

func (h *timelineSessionMemory) snapshot() []any {
	if h == nil {
		return []any{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.collectLocked()
	return cloneCompressionMemoryEntities(h.items)
}

// Only a new compression waits for earlier memory tails, under its own context.
// Ordinary prompt assembly, writes and the published summary remain unblocked.
func (h *timelineSessionMemory) wait(ctx context.Context) error {
	return h.waitWithProgress(ctx, nil)
}

func (h *timelineSessionMemory) waitWithProgress(ctx context.Context, onWait func()) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	pending := append([]*timelineCompressionCompletion(nil), h.pending...)
	h.mu.Unlock()
	var notified bool
	wait := func(done <-chan struct{}) error {
		select {
		case <-done:
			return nil
		default:
		}
		if !notified && onWait != nil {
			notified = true
			onWait()
		}
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for _, completion := range pending {
		if err := wait(completion.done); err != nil {
			return err
		}
		if completion.notified != nil {
			if err := wait(completion.notified); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *timelineSessionMemory) record(snapshot *timelineCompressionSnapshot) {
	completion := snapshot.MemoryCompletion
	if h == nil || completion == nil {
		return
	}
	h.mu.Lock()
	job := newTimelineMemoryRetry(snapshot)
	completion.retryKey = job.Key
	h.retries[job.Key] = job
	h.pending = append(h.pending, completion)
	h.mu.Unlock()
	go func() {
		<-completion.done
		h.mu.Lock()
		h.collectLocked()
		h.mu.Unlock()
		// Persist after the tail as the main loop may have saved its summary
		// already. Use the owning session, even when a task fork produced it.
		h.saveOwner()
	}()
}

func (h *timelineSessionMemory) saveOwner() error {
	if h != nil && h.owner != nil {
		h.mu.Lock()
		config := h.persistenceConfig
		h.mu.Unlock()
		if config == nil {
			h.owner.mu.RLock()
			config = h.owner.config
			h.owner.mu.RUnlock()
		}
		if cfg, ok := config.(*Config); ok && !cfg.DisableCreateDBRuntime && cfg.PersistentSessionId != "" {
			return h.owner.saveChecked(cfg.GetDB(), cfg.PersistentSessionId)
		}
	}
	return nil
}

// A nested coordinator may reuse this Timeline but bind a shorter Config.
// Session compression/persistence keeps the original owning runtime; private
// forks continue to use their own caller and context.
func (m *Timeline) compressionCallerConfig() AICallerConfigIf {
	m.mu.RLock()
	config, history := m.config, m.sessionMemory
	m.mu.RUnlock()
	if history != nil && history.owner == m {
		history.mu.Lock()
		if history.persistenceConfig != nil {
			config = history.persistenceConfig
		}
		history.mu.Unlock()
	}
	return config
}

// GetSessionMemoryCandidates returns an owned copy of completed candidates from
// this session's compression history. It neither searches nor writes memory DBs.
func (m *Timeline) GetSessionMemoryCandidates() []any {
	if m == nil {
		return []any{}
	}
	m.mu.RLock()
	history := m.sessionMemory
	m.mu.RUnlock()
	return history.snapshot()
}

// Compression sees only content; extraction metadata stays in storage.
func renderSessionMemoryCandidates(items []any) string {
	var text strings.Builder
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		fmt.Fprintf(&text, "内容：\n%s\n\n", item["content"])
	}
	return text.String()
}
