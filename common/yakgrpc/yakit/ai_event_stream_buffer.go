package yakit

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// streamEventBuffer coalesces frequent stream-delta updates to reduce sqlite write-lock contention.
//
// Key idea:
//  1. The first fragment creates the row immediately (so queries can find it).
//  2. Subsequent fragments are buffered in-memory and flushed in batches by a background ticker
//     (and can also be force-flushed by callers before doing heavy reads).
//
// This is intentionally best-effort: it prefers reducing write frequency/lock time over strict
// "every fragment must be durable immediately" semantics.
type streamEventBuffer struct {
	enabled atomic.Bool

	// flushEvery is the interval for attempting batch flushes.
	flushEvery time.Duration
	// flushBatchBytes forces a flush when a single event's pending bytes exceed this value.
	flushBatchBytes int
	// entryIdleTTL controls cleanup of idle entries with no pending bytes.
	entryIdleTTL time.Duration

	started atomic.Bool
	stopMu  sync.Mutex
	stopCh  chan struct{}

	entries *omap.OrderedMap[string, *streamEventBufferEntry]

	entryCount atomic.Int64
}

type streamEventBufferEntry struct {
	eventUUID string

	db atomic.Value // *gorm.DB (last seen)

	ensureOnce sync.Once
	ensureErr  error

	mu        sync.Mutex
	pending   []byte
	lastWrite time.Time
	lastFlush time.Time
}

var globalStreamEventBuffer = func() *streamEventBuffer {
	flushEvery := 2 * time.Second
	flushBatchBytes := 32 * 1024

	b := &streamEventBuffer{
		flushEvery:      flushEvery,
		flushBatchBytes: flushBatchBytes,
		entryIdleTTL:    30 * time.Second,
		stopCh:          nil,
		entries:         omap.NewEmptyOrderedMap[string, *streamEventBufferEntry](),
	}
	b.enabled.Store(true)
	return b
}()

func (b *streamEventBuffer) start() {
	if !b.enabled.Load() {
		return
	}
	if b.started.CompareAndSwap(false, true) {
		b.stopMu.Lock()
		b.stopCh = make(chan struct{})
		stopCh := b.stopCh
		b.stopMu.Unlock()
		go b.loop(stopCh)
	}
}

func (b *streamEventBuffer) loop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(b.flushEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.flushDue()
		case <-stopCh:
			return
		}
	}
}

func (b *streamEventBuffer) getEntry(eventUUID string) *streamEventBufferEntry {
	if b.entries != nil {
		if v, ok := b.entries.Get(eventUUID); ok {
			return v
		}
	}

	entry := &streamEventBufferEntry{eventUUID: eventUUID}
	if b.entries == nil {
		// Should never happen, but keep safe defaults.
		return entry
	}
	actual := b.entries.GetOrSet(eventUUID, entry)
	if actual == entry {
		b.entryCount.Add(1)
	}
	return actual
}

func (b *streamEventBuffer) deleteEntry(eventUUID string) {
	if b.entries == nil {
		b.stopIfIdle()
		return
	}
	if _, ok := b.entries.Get(eventUUID); ok {
		b.entries.Delete(eventUUID)
		b.entryCount.Add(-1)
	}
	b.stopIfIdle()
}

func (b *streamEventBuffer) stopIfIdle() {
	if !b.enabled.Load() {
		return
	}
	if b.entryCount.Load() != 0 {
		return
	}
	if !b.started.Load() {
		return
	}

	b.stopMu.Lock()
	ch := b.stopCh
	if ch != nil {
		close(ch)
		b.stopCh = nil
	}
	b.stopMu.Unlock()
	b.started.Store(false)
}

func (b *streamEventBuffer) append(outDb *gorm.DB, event *schema.AiOutputEvent) error {
	if !b.enabled.Load() || event == nil || event.EventUUID == "" {
		return b.appendDirect(outDb, event)
	}

	b.start()

	entry := b.getEntry(event.EventUUID)
	entry.db.Store(outDb)

	createdThisCall := false
	ranEnsure := false
	entry.ensureOnce.Do(func() {
		ranEnsure = true
		created, err := ensureStreamEventBase(outDb, event)
		createdThisCall = created
		entry.ensureErr = err
	})
	if entry.ensureErr != nil {
		return entry.ensureErr
	}

	// If this call created the row, the first fragment is already persisted by saveAIEvent().
	// Avoid double-appending.
	if ranEnsure && createdThisCall {
		// Mark activity so idle cleanup can evict this entry even if no further deltas arrive.
		entry.mu.Lock()
		entry.lastWrite = time.Now()
		entry.mu.Unlock()
		return nil
	}

	if len(event.StreamDelta) == 0 {
		return nil
	}

	now := time.Now()
	entry.mu.Lock()
	entry.pending = append(entry.pending, event.StreamDelta...)
	entry.lastWrite = now
	pendingLen := len(entry.pending)
	entry.mu.Unlock()

	// Backpressure: if the buffer for a single event gets too large, flush synchronously.
	if pendingLen >= b.flushBatchBytes {
		return b.flushEntry(entry, false)
	}
	return nil
}

func (b *streamEventBuffer) flushDue() {
	now := time.Now()
	if b.entries == nil {
		b.stopIfIdle()
		return
	}

	for _, key := range b.entries.Keys() {
		entry, ok := b.entries.Get(key)
		if !ok || entry == nil {
			continue
		}
		entry.mu.Lock()
		pendingLen := len(entry.pending)
		lastWrite := entry.lastWrite
		entry.mu.Unlock()

		// Cleanup idle entries to avoid unbounded growth.
		if pendingLen == 0 && !lastWrite.IsZero() && now.Sub(lastWrite) >= b.entryIdleTTL {
			b.deleteEntry(entry.eventUUID)
			continue
		}

		if pendingLen == 0 {
			continue
		}

		// Flush if the last write is older than flushEvery, or if it's been a while since last flush.
		if !lastWrite.IsZero() && now.Sub(lastWrite) >= b.flushEvery {
			_ = b.flushEntry(entry, true)
			continue
		}
	}
	b.stopIfIdle()
}

// FlushPendingStreamAIEvents force-flushes all buffered stream deltas.
// Callers that need read-your-writes semantics (e.g. query handlers) should call this before querying.
func FlushPendingStreamAIEvents() {
	_ = globalStreamEventBuffer.flushAll()
}

// FinishStreamAIEvent flushes and closes the buffered stream (if any) for the given event writer id.
// This is usually triggered by a structured event with node_id == "stream-finished".
func FinishStreamAIEvent(outDb *gorm.DB, eventWriterID string) {
	globalStreamEventBuffer.finish(outDb, eventWriterID)
}

func (b *streamEventBuffer) flushAll() error {
	if !b.enabled.Load() {
		return nil
	}
	var firstErr error
	if b.entries != nil {
		for _, key := range b.entries.Keys() {
			entry, ok := b.entries.Get(key)
			if !ok || entry == nil {
				continue
			}
			if err := b.flushEntry(entry, true); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (b *streamEventBuffer) finish(outDb *gorm.DB, eventWriterID string) {
	if !b.enabled.Load() || strings.TrimSpace(eventWriterID) == "" {
		return
	}
	if b.entries == nil {
		return
	}
	entry, ok := b.entries.Get(eventWriterID)
	if !ok || entry == nil {
		return
	}
	if outDb != nil {
		// Prefer latest db handle (dialect, session options).
		entry.db.Store(outDb)
	}

	// Try a strict flush; if locked, keep for background retry (best-effort).
	if err := b.flushEntry(entry, false); err != nil {
		if isDatabaseLockedErr(err) {
			_ = b.flushEntry(entry, true)
			return
		}
		return
	}

	entry.mu.Lock()
	empty := len(entry.pending) == 0
	entry.mu.Unlock()
	if empty {
		b.deleteEntry(eventWriterID)
	}
}

func (b *streamEventBuffer) flushEntry(entry *streamEventBufferEntry, isBestEffort bool) error {
	dbAny := entry.db.Load()
	db, _ := dbAny.(*gorm.DB)
	if db == nil {
		return nil
	}

	entry.mu.Lock()
	if len(entry.pending) == 0 {
		entry.mu.Unlock()
		return nil
	}
	chunk := make([]byte, len(entry.pending))
	copy(chunk, entry.pending)
	entry.pending = entry.pending[:0]
	entry.lastFlush = time.Now()
	entry.mu.Unlock()

	if err := appendStreamDelta(db, entry.eventUUID, chunk); err != nil {
		// For sqlite "database is locked" we re-queue and let next tick retry.
		if isBestEffort && isDatabaseLockedErr(err) {
			entry.mu.Lock()
			entry.pending = append(chunk, entry.pending...)
			entry.mu.Unlock()
			return nil
		}

		// Re-queue to avoid data loss; caller may decide to retry or flush later.
		entry.mu.Lock()
		entry.pending = append(chunk, entry.pending...)
		entry.mu.Unlock()

		if isBestEffort {
			log.Debugf("flush stream event pending bytes failed: event_uuid=%s err=%v", entry.eventUUID, err)
			return nil
		}
		return err
	}
	return nil
}

func (b *streamEventBuffer) appendDirect(outDb *gorm.DB, event *schema.AiOutputEvent) error {
	if outDb == nil || event == nil || event.EventUUID == "" {
		return nil
	}
	// Fallback to the original read-modify-write behavior.
	var existingEvent schema.AiOutputEvent
	if err := outDb.Where("event_uuid = ?", event.EventUUID).First(&existingEvent).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return saveAIEvent(outDb, event)
		}
		return err
	}
	existingEvent.StreamDelta = append(existingEvent.StreamDelta, event.StreamDelta...)
	return outDb.Save(&existingEvent).Error
}

func ensureStreamEventBase(outDb *gorm.DB, event *schema.AiOutputEvent) (created bool, _ error) {
	// Use FirstOrCreate pattern without transaction to avoid "database is locked" errors.
	var existingEvent schema.AiOutputEvent
	if err := outDb.Where("event_uuid = ?", event.EventUUID).First(&existingEvent).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return true, saveAIEvent(outDb, event)
		}
		return false, err
	}
	return false, nil
}

func appendStreamDelta(db *gorm.DB, eventUUID string, delta []byte) error {
	if len(delta) == 0 {
		return nil
	}

	// Optimized atomic append for sqlite (project DB). Fallback to read-modify-write on other dialects.
	if db != nil && db.Dialect() != nil {
		switch db.Dialect().GetName() {
		case "sqlite3":
			// X'' is an empty BLOB literal in SQLite.
			r := db.Model(&schema.AiOutputEvent{}).
				Where("event_uuid = ?", eventUUID).
				UpdateColumn("stream_delta", gorm.Expr("COALESCE(stream_delta, X'') || ?", delta))
			return r.Error
		case "mysql":
			r := db.Model(&schema.AiOutputEvent{}).
				Where("event_uuid = ?", eventUUID).
				UpdateColumn("stream_delta", gorm.Expr("CONCAT(COALESCE(stream_delta, ''), ?)", delta))
			return r.Error
		}
	}

	// Generic fallback.
	var existingEvent schema.AiOutputEvent
	if err := db.Where("event_uuid = ?", eventUUID).First(&existingEvent).Error; err != nil {
		return err
	}
	existingEvent.StreamDelta = append(existingEvent.StreamDelta, delta...)
	return db.Save(&existingEvent).Error
}

func isDatabaseLockedErr(err error) bool {
	if err == nil {
		return false
	}
	// sqlite3 driver error string
	return strings.Contains(err.Error(), "database is locked")
}
