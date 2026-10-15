package aicommon

import (
	"fmt"
	"time"
)

const (
	TimelineCompressionMaxInputTokens   = 512 * 1024
	TimelineCompressionMaxSummaryTokens = 16 * 1024
	timelineCompressionRetryDelay       = time.Minute
	timelineCompressionTargetDivisor    = 6
)

// CompressBeforePrompt owns scheduling: normalize, reserve, measure the actual
// main-model material, decide whether to start, then release and notify.
// A started optimization uses a lower target, never the trigger threshold.
func (m *Timeline) CompressBeforePrompt(options TimelineCompressionOptions) (*TimelineCompressionResult, error) {
	if m == nil {
		return nil, nil
	}
	options = m.prepareCompressionOptions(options)
	if options.MaxInputTokens == 0 {
		options.MaxInputTokens = TimelineCompressionMaxInputTokens
	}
	if options.MaxSummaryTokens == 0 {
		options.MaxSummaryTokens = TimelineCompressionMaxSummaryTokens
	}
	if err := m.awaitCompression(options); err != nil {
		return nil, err
	}
	var commits []*timelineCompressionCommit
	defer func() { m.finishCompression(commits) }()

	usage, err := m.measureTimelinePrompt(options.PromptOptions)
	if err != nil {
		return nil, err
	}
	if usage.Limit <= 0 || usage.Tokens < usage.Limit || m.compressionCoolingDown(usage) {
		return nil, nil
	}
	commits, err = m.foldThenCompressTimelineToTarget(options, usage.Limit)
	m.recordCompressionOutcome(options, usage, err)
	return collectCompressionResults(commits), err
}

// Fold redundant operations once without AI. If that already reaches the lower
// target, stop. Otherwise make exactly one AI compression and return its result,
// even when the rendered prompt is still above the target. The next check owns
// another round. The caller owns execution and observer delivery.
func (m *Timeline) foldThenCompressTimelineToTarget(options TimelineCompressionOptions, limit int64) (commits []*timelineCompressionCommit, err error) {
	options.TargetTokens = max(int64(1), limit/timelineCompressionTargetDivisor)
	if err := options.Context.Err(); err != nil {
		return commits, err
	}
	options.reportProgress(TimelineCompressionOptimizing)
	folded, err := m.foldTimelineWithoutAI(options.Context)
	if folded != nil {
		commits = append(commits, folded)
	}
	if err != nil {
		return commits, err
	}
	usage, err := m.measureCompressionTarget(options, limit)
	if err != nil || usage.Tokens <= options.TargetTokens {
		return commits, err
	}
	reduced, err := m.compressTimelineOnceWithAI(options)
	if reduced != nil {
		commits = append(commits, reduced)
	}
	if err != nil {
		return commits, err
	}
	// Detect a limit change after the model returns. A result still above the
	// target is a completed round, not a reason to call the model again.
	_, err = m.measureCompressionTarget(options, limit)
	return commits, err
}

func (m *Timeline) measureCompressionTarget(options TimelineCompressionOptions, limit int64) (timelinePromptUsage, error) {
	usage, err := m.measureTimelinePrompt(options.PromptOptions)
	if err == nil && usage.Limit > 0 && usage.Limit != limit {
		// Do not silently switch targets halfway through an optimization.
		return usage, fmt.Errorf("%w: %d -> %d", errTimelineCompressionLimitChanged, limit, usage.Limit)
	}
	return usage, err
}
