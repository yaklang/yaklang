package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

// The immutable result is published by closing done. Summary consumers never
// wait for the memory tail; memory observers read this only after the commit.
type timelineCompressionCompletion struct {
	done     chan struct{}
	notified chan struct{}
	retryKey string
	output   *timelineCompressionOutput
	err      error
}

func validateCompressionSelection(output *timelineCompressionOutput, snapshot *timelineCompressionSnapshot, limit TimelineCompressionOptions) error {
	if output == nil {
		return fmt.Errorf("timeline compression requires a summary object")
	}
	// A deletion-only pass has no ordinary history to summarize; its summary
	// may stay empty. Every other pass still requires a non-empty summary.
	deletionOnly := snapshot != nil && !snapshot.hasSummarySource() && snapshot.hasPromotedItems()
	if strings.TrimSpace(output.Summary) == "" && !deletionOnly {
		return fmt.Errorf("timeline compression requires a non-empty summary string")
	}
	if strings.Contains(output.Summary, aiprojection.Nonce()) {
		return fmt.Errorf("timeline compression returned a projection control token")
	}
	selection := *output
	selection.MemoryEntities = nil
	if limit.MaxSummaryTokens > 0 && TokenCountExceeds(selection.budgetText(snapshot), limit.MaxSummaryTokens) {
		return fmt.Errorf("timeline compression summary and retained items exceed safety limit %d", limit.MaxSummaryTokens)
	}
	return nil
}
