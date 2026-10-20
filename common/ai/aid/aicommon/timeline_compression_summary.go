package aicommon

import (
	"strings"
)

func compressionIndexLocator(text string) string {
	line := ""
	for _, candidate := range strings.Split(text, "\n") {
		candidate = strings.Join(strings.Fields(candidate), " ")
		// Snapshot bodies prefix the original with an ITEM_ID header that the
		// main-loop timeline does not render. Locate the visible text instead.
		if candidate == "" || strings.HasPrefix(candidate, "ITEM_ID=") {
			continue
		}
		line = candidate
		break
	}
	runes := []rune(line)
	if len(runes) > 80 {
		line = string(runes[:80]) + "…"
	}
	return line
}

func publishCompressionModelOutput(snapshot *timelineCompressionSnapshot, output *timelineCompressionOutput) {
	completion := &timelineCompressionCompletion{
		done:     make(chan struct{}),
		notified: make(chan struct{}),
		output:   output,
	}
	close(completion.done)
	snapshot.MemoryCompletion = completion
	snapshot.Output = &timelineCompressionOutput{
		Summary:                output.Summary,
		RetainedRange:          output.RetainedRange,
		RetainedIDs:            append([]int64(nil), output.RetainedIDs...),
		RetiredPromotedEntries: append([]int64(nil), output.RetiredPromotedEntries...),
	}
}
