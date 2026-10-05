package aicommon

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

// The immutable result is published by closing done. Summary consumers never
// wait for the memory tail; memory observers read this only after the commit.
type timelineCompressionCompletion struct {
	done   chan struct{}
	output *timelineCompressionOutput
	err    error
}

func validateCompressionSelection(output *timelineCompressionOutput, snapshot *timelineCompressionSnapshot, limit TimelineCompressionOptions) error {
	if output == nil || strings.TrimSpace(output.Summary) == "" {
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

func (m *Timeline) startCompressionResponse(snapshot *timelineCompressionSnapshot, limit TimelineCompressionOptions, requestPrompt string) (<-chan *timelineCompressionOutput, *timelineCompressionCompletion) {
	ready := make(chan *timelineCompressionOutput, 1)
	completion := &timelineCompressionCompletion{done: make(chan struct{})}
	ctx := limit.Context
	if ctx == nil {
		ctx = m.config.GetContext()
	}
	config := m.config
	var publish sync.Once
	var published *timelineCompressionOutput
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				completion.err = fmt.Errorf("timeline compression response panicked: %v", recovered)
			}
			close(completion.done)
		}()
		completion.err = fmt.Errorf("timeline compression skipped or returned no result")
		config.ScheduleAuxiliaryTask(ctx, CallerLabelTimelineCompress,
			func() string { return requestPrompt },
			func(action *Action) {
				raw, _ := action.LookupCanonicalParam("summary")
				text, ok := raw.(string)
				if !ok {
					completion.err = fmt.Errorf("timeline compression summary must be a root string field")
					return
				}
				output := parseTimelineCompressionOutput(action, snapshot, strings.TrimSpace(text))
				if err := validateCompressionSelection(output, snapshot, limit); err != nil {
					completion.err = err
					return
				}
				if published != nil && (published.Summary != output.Summary || published.RetainedRange != output.RetainedRange) {
					completion.err = fmt.Errorf("timeline compression selection changed after publication; memory discarded")
					return
				}
				completion.output, completion.err = output, nil
				if limit.MaxSummaryTokens > 0 && TokenCountExceeds(output.budgetText(snapshot), limit.MaxSummaryTokens) {
					completion.output.MemoryEntities = nil
					completion.err = fmt.Errorf("timeline compression memory exceeds safety limit %d", limit.MaxSummaryTokens)
				}
			},
			WithAuxiliaryOutputSchema("timeline-summary", timelineCompressionSchema),
			WithAuxiliaryResponseHandler(func(response *AIResponse) (*Action, error) {
				action := NewActionMaker("timeline-summary", WithActionRootFieldsCallback(func(fields map[string]any) {
					summary, summarySeen := fields["summary"].(string)
					rawRange, rangeSeen := fields["ratain_timeline_item_range"]
					if !summarySeen || !rangeSeen {
						return
					}
					output := compressionSelection(snapshot, summary, rawRange)
					if validateCompressionSelection(output, snapshot, limit) != nil {
						return
					}
					publish.Do(func() { published = output; ready <- output })
				})).ReadFromReader(ctx, response.GetOutputStreamReader("timeline-compression", true, config.GetEmitter()))
				if err := action.WaitParseResult(ctx); err != nil {
					return nil, err
				}
				action.WaitStream(ctx)
				raw, _ := action.LookupCanonicalParam("summary")
				text, ok := raw.(string)
				if !ok || strings.TrimSpace(text) == "" {
					return nil, fmt.Errorf("timeline compression requires a non-empty summary string")
				}
				return action, nil
			}),
			WithAuxiliaryOnError(func(err error) { completion.err = fmt.Errorf("timeline compression request failed: %w", err) }),
			WithAuxiliaryOpts(WithLiteForgeDisableTimeline(), WithLiteForgeStaticInstruction(timelineCompressionInstruction),
				WithLiteForgeMaxPromptTokens(limit.MaxInputTokens), WithGeneralConfigExtraRequestOpts(WithAIRequest_CallerLabel(CallerLabelTimelineCompress))),
		)
	}()
	return ready, completion
}
