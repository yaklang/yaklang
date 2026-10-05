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
	mu      sync.Mutex
	owner   *Timeline
	items   []any
	seen    map[string]bool
	pending []*timelineCompressionCompletion
}

func newTimelineSessionMemory(owner *Timeline, items []any) *timelineSessionMemory {
	h := &timelineSessionMemory{owner: owner, seen: make(map[string]bool)}
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
	if h == nil {
		return nil
	}
	h.mu.Lock()
	pending := append([]*timelineCompressionCompletion(nil), h.pending...)
	h.mu.Unlock()
	for _, completion := range pending {
		select {
		case <-completion.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (h *timelineSessionMemory) record(completion *timelineCompressionCompletion) {
	if h == nil || completion == nil {
		return
	}
	h.mu.Lock()
	h.pending = append(h.pending, completion)
	h.mu.Unlock()
	go func() {
		<-completion.done
		h.mu.Lock()
		h.collectLocked()
		h.mu.Unlock()
		if completion.err != nil || completion.output == nil || len(completion.output.MemoryEntities) == 0 {
			return
		}
		// Persist after the tail as the main loop may have saved its summary
		// already. Use the owning session, even when a task fork produced it.
		owner := h.owner
		if owner != nil {
			if cfg, ok := owner.config.(*Config); ok && !cfg.DisableCreateDBRuntime && cfg.PersistentSessionId != "" {
				owner.Save(cfg.GetDB(), cfg.PersistentSessionId)
			}
		}
	}()
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

// Compression sees only titles and content; extraction metadata stays in storage.
func renderSessionMemoryCandidates(items []any) string {
	var text strings.Builder
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		fmt.Fprintf(&text, "标题：%s\n内容：\n%s\n\n", item["title"], item["content"])
	}
	return text.String()
}
