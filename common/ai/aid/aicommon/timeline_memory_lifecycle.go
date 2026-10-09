package aicommon

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// A durable extraction source, independent of ordinary prompt history. A
// published summary may retire originals before its memory tail arrives.
// Failed/interrupted tails and private agent tails keep this source for replay.
type timelineMemoryRetry struct {
	Key          string                       `json:"key"`
	Snapshot     *timelineCompressionSnapshot `json:"source"`
	CoveredState string                       `json:"covered_state,omitempty"`
}

func newTimelineMemoryRetry(snapshot *timelineCompressionSnapshot) timelineMemoryRetry {
	copy := *snapshot
	copy.Context, copy.MemoryCompletion, copy.Output, copy.NotifyCommitted = nil, nil, nil, nil
	copy.Head = cloneTimelineCompressedHead(snapshot.Head)
	copy.Items = append([]timelineCompressionSnapshotItem(nil), snapshot.Items...)
	copy.ExactItemIDs = append([]int64(nil), snapshot.ExactItemIDs...)
	copy.UserContexts = append([]timelineCompressionUserContext(nil), snapshot.UserContexts...)
	copy.SessionMemoryCandidates = cloneCompressionMemoryEntities(snapshot.SessionMemoryCandidates)
	copy.RetainedContext = make(map[string]string, len(snapshot.RetainedContext))
	for key, value := range snapshot.RetainedContext {
		copy.RetainedContext[key] = value
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(snapshot.SourceState+renderCompressionSnapshotItems(snapshot.Items))))
	return timelineMemoryRetry{Key: key, Snapshot: &copy}
}

func (h *timelineSessionMemory) retrySnapshot() []timelineMemoryRetry {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.collectLocked()
	var keys []string
	for key := range h.retries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var jobs []timelineMemoryRetry
	for _, key := range keys {
		job := h.retries[key]
		if job.Snapshot != nil {
			copy := newTimelineMemoryRetry(job.Snapshot)
			copy.Key, copy.CoveredState = job.Key, job.CoveredState
			jobs = append(jobs, copy)
		}
	}
	return jobs
}

// Called under Timeline.mu. Logical business data only: freeze boundaries,
// thoughts, iteration bookkeeping and persistence progress cannot make it dirty.
func (m *Timeline) memoryFingerprintLocked() string {
	snapshot, err := m.captureCompressionSnapshotLocked()
	if err != nil {
		return "invalid-source"
	}
	return compressionMemoryFingerprint(snapshot)
}

func compressionMemoryFingerprint(snapshot *timelineCompressionSnapshot) string {
	var users []string
	for _, entry := range snapshot.UserContexts {
		users = append(users, entry.Text)
	}
	previous := ""
	if snapshot.Head != nil {
		previous = snapshot.Head.Text
	}
	var bodies []string
	for _, item := range snapshot.Items {
		if item.PromptText != "" {
			_, body, _ := strings.Cut(item.PromptText, "\n")
			bodies = append(bodies, body)
		}
	}
	history := strings.Join(bodies, "\n")
	if len(users) == 0 && snapshot.Evidence == "" && previous == "" && history == "" {
		return ""
	}
	raw, _ := json.Marshal([]any{users, snapshot.Evidence, previous, history})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

// RequestMemoryFinalization journals intent, not a model request. Cancellation
// and gRPC disconnect retain it; only a full user-task boundary may finalize it.
func (m *Timeline) RequestMemoryFinalization() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	m.memoryFinalizePending = true
	m.mu.Unlock()
	return m.sessionMemory.saveOwner()
}

func (m *Timeline) HasPendingMemoryFinalization() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.memoryFinalizePending
}

// NeedsMemoryFinalization includes new business data and unresolved save/tail
// work. A repeated end with no changes avoids both model and storage requests.
func (m *Timeline) NeedsMemoryFinalization() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.memoryFingerprintLocked()
	if m.memoryFinalizePending || (state != "" && state != m.memoryCoveredState) {
		return true
	}
	h := m.sessionMemory
	h.mu.Lock()
	defer h.mu.Unlock()
	h.collectLocked()
	return len(h.retries) > 0 || len(h.pending) > 0
}

// Called only after durable memory persistence succeeds. New input arriving
// during finalization stays pending for a later boundary/recovery.
func (m *Timeline) FinishMemoryFinalization() error {
	m.mu.Lock()
	state := m.memoryFingerprintLocked()
	if state == "" || state == m.memoryCoveredState {
		m.memoryFinalizePending = false
	}
	m.mu.Unlock()
	if err := m.sessionMemory.saveOwner(); err != nil {
		m.mu.Lock()
		m.memoryFinalizePending = true
		m.mu.Unlock()
		return err
	}
	return nil
}

// ArchiveMemoryBranch preserves an unmerged private tail for session memory
// extraction without importing the child's intermediate history into prompts.
// Releasing it twice is idempotent. No AI is invoked at a branch boundary.
func (m *Timeline) ArchiveMemoryBranch(branch *Timeline, afterID int64) error {
	if m == nil || branch == nil || m == branch {
		return nil
	}
	snapshot, err := branch.captureCompressionSnapshot()
	if err != nil {
		return err
	}
	items := snapshot.Items[:0]
	for _, item := range snapshot.Items {
		if item.ID > afterID {
			items = append(items, item)
		}
	}
	snapshot.Items = items
	if branch.sessionMemory != m.sessionMemory {
		// Clean agents own a separate ledger. Transfer completed candidates and
		// failed/pending compression sources before discarding that runtime.
		child := branch.sessionMemory
		child.mu.Lock()
		child.collectLocked()
		candidates := cloneCompressionMemoryEntities(child.items)
		jobs := make(map[string]timelineMemoryRetry, len(child.retries))
		processed := make(map[string]bool, len(child.processed))
		for key, job := range child.retries {
			jobs[key] = job
		}
		for key, done := range child.processed {
			processed[key] = done
		}
		pending := append([]*timelineCompressionCompletion(nil), child.pending...)
		child.mu.Unlock()
		h := m.sessionMemory
		h.mu.Lock()
		h.appendLocked(candidates)
		for key, job := range jobs {
			if !h.processed[key] {
				h.retries[key] = job
			}
		}
		for key, done := range processed {
			if done {
				h.processed[key] = true
			}
		}
		for _, completion := range pending {
			found := false
			for _, existing := range h.pending {
				if existing == completion {
					found = true
					break
				}
			}
			if !found {
				h.pending = append(h.pending, completion)
			}
		}
		h.mu.Unlock()
	}
	hasExactTail := false
	for _, entry := range snapshot.UserContexts {
		if entry.ID > afterID {
			hasExactTail = true
		}
	}
	if len(items) == 0 && (snapshot.Head == nil || snapshot.Head.CoveredEndItemID <= afterID) && !hasExactTail {
		return m.sessionMemory.saveOwner()
	}
	snapshot.FinalizingMemory = true
	snapshot.MemoryInputLimit, snapshot.MemoryOutputLimit = TimelineCompressionMaxInputTokens, TimelineCompressionMaxSummaryTokens
	job := newTimelineMemoryRetry(snapshot)
	h := m.sessionMemory
	h.mu.Lock()
	if !h.processed[job.Key] {
		h.retries[job.Key] = job
	}
	h.mu.Unlock()
	return h.saveOwner()
}

// FinalizeMemory drains completed/pending tails and reduces new business data
// once. The caller supplies a bounded lifecycle and persists candidates before
// FinishMemoryFinalization. Runtime-only locks and durable receipts are never
// used by prompt rendering.
func (m *Timeline) FinalizeMemory(ctx context.Context, limits ...TimelineCompressionOptions) error {
	var options TimelineCompressionOptions
	if len(limits) > 0 {
		options = limits[0]
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil {
		return nil
	}
	h := m.sessionMemory
	select {
	case h.work <- struct{}{}:
		defer func() { <-h.work }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := h.wait(ctx); err != nil {
		return err
	}
	if err := m.replayMemorySources(ctx); err != nil {
		return err
	}
	for {
		m.mu.Lock()
		if m.compressing {
			done := m.compressionDone
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		state := m.memoryFingerprintLocked()
		covered := m.memoryCoveredState
		m.mu.Unlock()
		if state == "" || state == covered {
			return h.saveOwner()
		}
		options.Context, options.FinalizeMemory = ctx, true
		if options.MaxInputTokens <= 0 {
			options.MaxInputTokens = TimelineCompressionMaxInputTokens
		}
		if options.MaxSummaryTokens <= 0 {
			options.MaxSummaryTokens = TimelineCompressionMaxSummaryTokens
		}
		if _, err := m.CompressOnce(options); err != nil {
			if err == errTimelineCompressionBusy {
				continue
			}
			return err
		}
		if err := h.wait(ctx); err != nil {
			return err
		}
		return h.saveOwner()
	}
}

// PrepareMemoryRecovery captures only the restored pending source. Later user
// input cannot leak into this replay, which never compresses a live task.
func (m *Timeline) PrepareMemoryRecovery() error {
	m.mu.Lock()
	if !m.memoryFinalizePending {
		m.mu.Unlock()
		return nil
	}
	snapshot, err := m.captureCompressionSnapshotLocked()
	if err != nil {
		m.mu.Unlock()
		return err
	}
	state := compressionMemoryFingerprint(snapshot)
	covered := m.memoryCoveredState
	m.mu.Unlock()
	if state == "" || state == covered {
		return nil
	}
	snapshot.FinalizingMemory = true
	job := newTimelineMemoryRetry(snapshot)
	job.CoveredState = state
	h := m.sessionMemory
	h.mu.Lock()
	if !h.processed[job.Key] {
		h.retries[job.Key] = job
	}
	h.mu.Unlock()
	return h.saveOwner()
}

// RecoverMemory only replays archived sources; it never reads the live tail.
func (m *Timeline) RecoverMemory(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h := m.sessionMemory
	select {
	case h.work <- struct{}{}:
		defer func() { <-h.work }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := h.wait(ctx); err != nil {
		return err
	}
	return m.replayMemorySources(ctx)
}

func (m *Timeline) replayMemorySources(ctx context.Context) error {
	h := m.sessionMemory
	for _, job := range h.retrySnapshot() {
		h.mu.Lock()
		processed := h.processed[job.Key]
		h.mu.Unlock()
		if processed {
			continue
		}
		snapshot := job.Snapshot
		snapshot.SessionMemoryCandidates = h.snapshot()
		limits := TimelineCompressionOptions{Context: ctx, MaxInputTokens: snapshot.MemoryInputLimit, MaxSummaryTokens: snapshot.MemoryOutputLimit}
		if limits.MaxInputTokens <= 0 {
			limits.MaxInputTokens = TimelineCompressionMaxInputTokens
		}
		if limits.MaxSummaryTokens <= 0 {
			limits.MaxSummaryTokens = TimelineCompressionMaxSummaryTokens
		}
		if _, err := m.summarizeCompressionSnapshot(snapshot, limits); err != nil {
			return err
		}
		completion := snapshot.MemoryCompletion
		select {
		case <-completion.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		if completion.err != nil {
			return completion.err
		}
		entities := completion.output.MemoryEntities
		h.mu.Lock()
		h.appendLocked(cloneCompressionMemoryEntities(entities))
		if len(entities) == 0 {
			delete(h.retries, job.Key)
		}
		h.processed[job.Key] = true
		callbacks := h.callbacks.snapshot()
		h.mu.Unlock()
		if job.CoveredState != "" {
			m.mu.Lock()
			m.memoryCoveredState = job.CoveredState
			m.mu.Unlock()
		}
		if err := h.saveOwner(); err != nil {
			return err
		}
		completion.retryKey = job.Key
		h.mu.Lock()
		h.pending = append(h.pending, completion)
		h.mu.Unlock()
		go func() {
			defer close(completion.notified)
			for _, callback := range callbacks {
				invokeTimelineCallback(callback, TimelineMemoryEvent{ThroughID: snapshot.ThroughID, Prompt: snapshot.SummaryPrompt,
					Instruction: timelineCompressionInstruction, MemoryEntities: cloneCompressionMemoryEntities(entities)})
			}
		}()
		select {
		case <-completion.notified:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (h *timelineSessionMemory) processedSnapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.collectLocked()
	keys := make([]string, 0, len(h.processed))
	for key := range h.processed {
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// A commit may race newer input. Cover only the snapshot that actually reached
// the model, not newer Open entries/evidence appended while it was running.
func (m *Timeline) markCompressionMemoryCoveredLocked(source *timelineCompressionSnapshot) {
	current, err := m.captureCompressionSnapshotLocked()
	if err != nil {
		return
	}
	items := current.Items[:0]
	for _, item := range current.Items {
		if item.ID <= source.ThroughID {
			items = append(items, item)
		}
	}
	current.Items = items
	current.Evidence = source.Evidence
	current.UserContexts = source.UserContexts
	m.memoryCoveredState = compressionMemoryFingerprint(current)
}

// GetMemoryPersistenceBatch captures candidates and their completed extraction
// sources together. Sources are retired only after this batch is durably saved.
func (m *Timeline) GetMemoryPersistenceBatch() ([]any, []string) {
	h := m.sessionMemory
	h.mu.Lock()
	defer h.mu.Unlock()
	h.collectLocked()
	var sources []string
	for key := range h.retries {
		if h.processed[key] {
			sources = append(sources, key)
		}
	}
	sort.Strings(sources)
	return cloneCompressionMemoryEntities(h.items), sources
}

func (m *Timeline) AcknowledgeMemoryPersistence(sources []string) error {
	h := m.sessionMemory
	h.mu.Lock()
	retired := make(map[string]timelineMemoryRetry)
	for _, key := range sources {
		if h.processed[key] {
			if source, exists := h.retries[key]; exists {
				retired[key] = source
				delete(h.retries, key)
			}
		}
	}
	h.mu.Unlock()
	if err := h.saveOwner(); err != nil {
		h.mu.Lock()
		for key, source := range retired {
			h.retries[key] = source
		}
		h.mu.Unlock()
		return err
	}
	return nil
}

func (m *Timeline) checkpointCompressionMemory(snapshot *timelineCompressionSnapshot) error {
	if snapshot.MemoryCompletion == nil {
		return m.sessionMemory.saveOwner()
	}
	m.mu.Lock()
	_, err := m.validateCompressionSourceLocked(snapshot)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	h := m.sessionMemory
	job := newTimelineMemoryRetry(snapshot)
	h.mu.Lock()
	if !h.processed[job.Key] {
		h.retries[job.Key] = job
	}
	h.mu.Unlock()
	return h.saveOwner()
}
