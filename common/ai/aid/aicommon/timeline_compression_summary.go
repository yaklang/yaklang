package aicommon

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"strings"
	"text/template"
)

//go:embed prompts/timeline/compression.txt
var timelineCompressionTemplate string

//go:embed prompts/timeline/compression.json
var timelineCompressionSchema string

// renderCompressionSummaryPrompt renders one complete reduction request. Native
// replay is historical data here, not messages to execute/project in this helper.
// Redact the process nonce on this request-only copy, then JSON-encode the source
// so embedded tags cannot become projection controls or source delimiters.
// The output schema is supplied once by the auxiliary scheduler.
func renderCompressionSummaryPrompt(snapshot *timelineCompressionSnapshot) (string, error) {
	if snapshot == nil {
		return "", fmt.Errorf("invalid timeline compression snapshot")
	}
	previous := ""
	if snapshot.Head != nil {
		previous = snapshot.Head.Text
	}
	history := renderCompressionSnapshotItems(snapshot.Items)
	if strings.TrimSpace(previous) == "" && strings.TrimSpace(history) == "" {
		return "", fmt.Errorf("timeline compression has no history to summarize")
	}
	source, err := json.MarshalIndent(struct {
		RetainedContext map[string]string `json:"retained_context,omitempty"`
		PreviousSummary string            `json:"previous_summary"`
		OlderHistory    string            `json:"history_to_summarize"`
	}{snapshot.RetainedContext, aiprojection.RedactNonce(previous), aiprojection.RedactNonce(history)}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode compression source: %w", err)
	}
	tmpl, err := template.New("timeline-compression").Parse(timelineCompressionTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Source string
	}{aiprojection.RedactNonce(string(source))})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// summarizeCompressionSnapshot schedules one complete reduction for the
// transaction before prompt assembly, without chunks, refinement or
// truncation. Transport/parser retries still follow the existing Config policy.
// It never commits, freezes, promotes or retires any Timeline content.
func (m *Timeline) summarizeCompressionSnapshot(snapshot *timelineCompressionSnapshot, limits ...TimelineCompressionOptions) (string, error) {
	if m == nil || m.config == nil {
		return "", fmt.Errorf("timeline compression requires an auxiliary scheduler")
	}
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	if err != nil {
		return "", err
	}
	// Capture the safety limits before invoking callbacks; don't reread mutable state
	// or the caller's snapshot after the request has started.
	var limit TimelineCompressionOptions
	if len(limits) > 0 {
		limit = limits[0]
	}
	ctx := limit.Context
	if ctx == nil {
		ctx = m.config.GetContext()
	}
	if limit.MaxInputTokens > 0 && TokenCountExceeds(prompt+"\n"+timelineCompressionSchema, limit.MaxInputTokens) {
		return "", fmt.Errorf("timeline compression input exceeds safety limit %d; source preserved", limit.MaxInputTokens)
	}
	var summary string
	resultErr := fmt.Errorf("timeline compression skipped or returned no result")
	m.config.ScheduleAuxiliaryTask(ctx, CallerLabelTimelineCompress,
		func() string { return prompt },
		func(action *Action) {
			if action == nil {
				resultErr = fmt.Errorf("timeline compression returned no action")
				return
			}
			if err := action.WaitParseResult(ctx); err != nil {
				resultErr = fmt.Errorf("parse timeline compression: %w", err)
				return
			}
			if !action.ValidCheck("timeline-summary") {
				resultErr = fmt.Errorf("timeline compression returned an unexpected action")
				return
			}
			raw, exists := action.LookupCanonicalParam("summary")
			text, isString := raw.(string)
			if !exists || !isString {
				resultErr = fmt.Errorf("timeline compression summary must be a root string field")
				return
			}
			text = strings.TrimSpace(text)
			if text == "" {
				resultErr = fmt.Errorf("timeline compression returned an empty summary")
				return
			}
			if strings.Contains(text, aiprojection.Nonce()) {
				resultErr = fmt.Errorf("timeline compression returned a projection control token")
				return
			}
			if limit.MaxSummaryTokens > 0 && TokenCountExceeds(text, limit.MaxSummaryTokens) {
				resultErr = fmt.Errorf("timeline compression summary exceeds safety limit %d", limit.MaxSummaryTokens)
				return
			}
			summary, resultErr = text, nil
		},
		WithAuxiliaryOutputSchema("timeline-summary", timelineCompressionSchema),
		WithAuxiliaryOnError(func(err error) { resultErr = fmt.Errorf("timeline compression request failed: %w", err) }),
		WithAuxiliaryOpts(WithLiteForgeDisableTimeline(),
			WithLiteForgeMaxPromptTokens(limit.MaxInputTokens),
			WithGeneralConfigExtraRequestOpts(WithAIRequest_CallerLabel(CallerLabelTimelineCompress))),
	)
	return summary, resultErr
}
