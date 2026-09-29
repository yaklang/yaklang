package aicommon

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	TimelineCompressionMaxInputTokens   = 256 * 1024
	TimelineCompressionMaxSummaryTokens = 16 * 1024
	timelineCompressionRetryDelay       = time.Minute
)

// CompressBeforePrompt checks the threshold and synchronously compresses history
// before prompt assembly. Writes and reads never schedule AI or move freeze boundaries.
// A failed unchanged snapshot is
// not retried automatically; changed input gets a cooldown. CompressOnce remains
// the explicit retry path. Limits are safety caps, not target summary lengths.
func (m *Timeline) CompressBeforePrompt(options TimelineCompressionOptions) (*TimelineCompressionResult, error) {
	if m == nil {
		return nil, nil
	}
	ctx := options.Context
	if ctx == nil && m.config != nil {
		ctx = m.config.GetContext()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	options.Context = ctx
	if options.MaxInputTokens == 0 {
		options.MaxInputTokens = TimelineCompressionMaxInputTokens
	}
	if options.MaxSummaryTokens == 0 {
		options.MaxSummaryTokens = TimelineCompressionMaxSummaryTokens
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.mu.Lock()
		if m.compressing {
			done := m.compressionDone
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if m.totalDumpContentLimit <= 0 {
			m.mu.Unlock()
			return nil, nil
		}
		snapshot, err := m.captureCompressionSnapshotLocked()
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		limit := m.totalDumpContentLimit
		// Include pending precise deltas in the threshold, but not the already
		// promoted state that compression cannot reduce.
		var pending strings.Builder
		for _, id := range snapshot.ExactItemIDs {
			if id > snapshot.FrozenThroughID {
				item, _ := m.idToTimelineItem.Get(id)
				pending.WriteString(item.value.(*PromotableTimelineItem).OpenPromptText())
				pending.WriteByte('\n')
			}
		}
		fingerprint := fmt.Sprintf("%s/%v/%d", snapshot.SourceState, snapshot.Head, limit)
		snapshot.BeforePromptFingerprint, snapshot.BeforePromptLimit = fingerprint, limit
		if fingerprint == m.compressionLastFailure || time.Now().Before(m.compressionRetryAfter) {
			m.mu.Unlock()
			return nil, nil
		}
		m.mu.Unlock()
		_ = prepareCompressionSnapshot(snapshot)
		if (len(snapshot.Items) == 0 && pending.Len() == 0) || int64(MeasureTokens(snapshot.InputText+pending.String())) < limit {
			return nil, nil
		}
		result, err := m.compressOnce(options, snapshot)
		if err == errTimelineCompressionBusy || err == errTimelineCompressionBeforePromptChanged {
			continue
		}
		return result, err
	}
}
