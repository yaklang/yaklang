package aicommon

import (
	"context"
	"sort"
)

// A committed transaction and its detached observer receipt. The owner of the
// compression lifecycle dispatches receipts only after releasing execution.
type timelineCompressionCommit struct {
	Result        *TimelineCompressionResult
	Notify        func()
	FreezeVersion int64
}

func (m *Timeline) prepareCompressionOptions(options TimelineCompressionOptions) TimelineCompressionOptions {
	if options.Context == nil {
		if config := m.compressionCallerConfig(); config != nil {
			options.Context = config.GetContext()
		}
		if options.Context == nil {
			options.Context = context.Background()
		}
	}
	notify := options.OnProgress
	var lastStage TimelineCompressionStage
	options.OnProgress = func(stage TimelineCompressionStage) {
		if notify != nil && stage != lastStage {
			lastStage = stage
			notify(stage)
		}
	}
	return options
}

// Both public compression entries use the same reservation. Ordinary writes
// still acquire Timeline.mu normally and can append while the model runs.
func (m *Timeline) tryBeginCompression() (<-chan struct{}, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.compressing {
		return m.compressionDone, false
	}
	m.compressing = true
	m.compressionDone = make(chan struct{})
	return m.compressionDone, true
}

func (m *Timeline) awaitCompression(options TimelineCompressionOptions) error {
	for {
		if err := options.Context.Err(); err != nil {
			return err
		}
		done, acquired := m.tryBeginCompression()
		if acquired {
			return nil
		}
		options.reportProgress(TimelineCompressionWaiting)
		select {
		case <-done:
		case <-options.Context.Done():
			return options.Context.Err()
		}
	}
}

func (m *Timeline) finishCompression(commits []*timelineCompressionCommit) {
	m.mu.Lock()
	m.compressionSnapshot = nil
	m.compressing = false
	close(m.compressionDone)
	m.compressionDone = nil
	m.mu.Unlock()
	// Callbacks can reenter CompressBeforePrompt without waiting on themselves.
	for _, commit := range commits {
		if commit != nil && commit.Notify != nil {
			commit.Notify()
		}
	}
}

// Return the latest reduction's summary/selection and all actual retirements.
// A later lossless compaction must not replace the summary or input statistics
// with its empty bookkeeping result.
func collectCompressionResults(commits []*timelineCompressionCommit) *TimelineCompressionResult {
	var result *TimelineCompressionResult
	for _, commit := range commits {
		if commit == nil || commit.Result == nil {
			continue
		}
		if result == nil {
			result = &TimelineCompressionResult{}
		}
		next := commit.Result
		result.ThroughID = max(result.ThroughID, next.ThroughID)
		result.RetiredIDs = append(result.RetiredIDs, next.RetiredIDs...)
		result.RetiredPromotedEntryIDs = append(result.RetiredPromotedEntryIDs, next.RetiredPromotedEntryIDs...)
		if next.InputTokens > 0 || next.Summary != "" {
			result.Summary, result.InputTokens, result.SummaryTokens = next.Summary, next.InputTokens, next.SummaryTokens
			result.RetainedRange = next.RetainedRange
			result.RetainedIDs = append([]int64(nil), next.RetainedIDs...)
		}
	}
	if result != nil {
		result.RetiredIDs = sortedCompressionIDs(result.RetiredIDs)
		result.RetiredPromotedEntryIDs = sortedCompressionIDs(result.RetiredPromotedEntryIDs)
	}
	return result
}

func sortedCompressionIDs(ids []int64) []int64 {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	unique := ids[:0]
	for _, id := range ids {
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	return unique
}
