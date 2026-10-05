package aicommon

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"sort"
	"strings"
	"text/template"
)

var timelineCompressionTemplate = promptloader.MustLoad("ai/aid/aicommon/prompts/timeline/compression.txt")

var timelineCompressionInstruction = promptloader.MustLoad("ai/aid/aicommon/prompts/timeline/compression_instruction.txt")

var timelineCompressionSchema = promptloader.MustLoad("ai/aid/aicommon/prompts/timeline/compression.json")

// renderCompressionSummaryPrompt renders one complete reduction request. Native
// replay is historical data here, not messages to execute/project in this helper.
// Redact the process nonce on this request-only copy; stable data frames display
// raw text without JSON escaping or executable projection envelopes.
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
	if strings.TrimSpace(previous) == "" && strings.TrimSpace(history) == "" &&
		(!snapshot.FinalizingMemory || (len(snapshot.UserContexts) == 0 && strings.TrimSpace(snapshot.Evidence) == "")) {
		return "", fmt.Errorf("timeline compression has no history to summarize")
	}
	var source strings.Builder
	keys := []string{"user_query", "frozen_user_context", "todo"}
	labels := map[string]string{"user_query": "ORIGINAL_USER_QUERY", "frozen_user_context": "FROZEN_USER_CONTEXT", "todo": "TODO"}
	var extra []string
	for key := range snapshot.RetainedContext {
		// Framework instructions are not task history, including older callers.
		if key == "task_instruction" {
			continue
		}
		if _, known := labels[key]; !known {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)
	for _, key := range keys {
		label := labels[key]
		if label == "" {
			label = "RETAINED_CONTEXT"
		}
		content := snapshot.RetainedContext[key]
		if label == "RETAINED_CONTEXT" {
			content = key + ":\n" + content
		}
		if key == "frozen_user_context" && content == "" {
			for _, entry := range snapshot.UserContexts {
				if entry.Frozen {
					content += entry.Text + "\n"
				}
			}
		}
		source.WriteString(compressionSourceTag(label, content))
	}
	var openUser strings.Builder
	for _, entry := range snapshot.UserContexts {
		if !entry.Frozen {
			fmt.Fprintf(&openUser, "# item=%d\n%s\n", entry.ID, entry.Text)
		}
	}
	source.WriteString(compressionSourceTag("OPEN_USER_CONTEXT", openUser.String()))
	source.WriteString(compressionSourceTag("SESSION_EVIDENCE", snapshot.Evidence))
	source.WriteString(compressionSourceTag("SESSION_MEMORY_CANDIDATES", renderSessionMemoryCandidates(snapshot.SessionMemoryCandidates)))
	source.WriteString(compressionSourceTag("PREVIOUS_SUMMARY", previous))
	var frozen, open []timelineCompressionSnapshotItem
	for _, item := range snapshot.Items {
		if item.Frozen {
			frozen = append(frozen, item)
		} else {
			open = append(open, item)
		}
	}
	source.WriteString(compressionSourceTag("FROZEN_TIMELINE", renderCompressionSnapshotItems(frozen)))
	source.WriteString(compressionSourceTag("OPEN_TIMELINE", renderCompressionSnapshotItems(open)))
	tmpl, err := template.New("timeline-compression").Parse(timelineCompressionTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Source string
	}{source.String()})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Plain-text data frames have stable content hashes, not authenticated replay
// tags. Redacting historical projection nonces prevents their control envelopes
// from becoming outgoing assistant/tool messages in the compression helper.
func compressionSourceTag(label, content string) string {
	content = aiprojection.RedactNonce(content)
	hash := StablePromptNonce("timeline-compression", label, content)
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>\n\n", label, hash, content, label, hash)
}

// summarizeCompressionSnapshot schedules one complete reduction for the
// transaction before prompt assembly, without chunks, refinement or
// truncation. Transport/parser retries still follow the existing Config policy.
// It never commits, freezes, promotes or retires any Timeline content.
func (m *Timeline) summarizeCompressionSnapshot(snapshot *timelineCompressionSnapshot, limits ...TimelineCompressionOptions) (string, error) {
	if m == nil {
		return "", fmt.Errorf("timeline compression requires an auxiliary scheduler")
	}
	config := m.compressionCallerConfig()
	if config == nil {
		return "", fmt.Errorf("timeline compression requires an auxiliary scheduler")
	}
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	if err != nil {
		return "", err
	}
	snapshot.SummaryPrompt = prompt
	// Capture the safety limits before invoking callbacks; don't reread mutable state
	// or the caller's snapshot after the request has started.
	var limit TimelineCompressionOptions
	if len(limits) > 0 {
		limit = limits[0]
	}
	snapshot.MemoryInputLimit, snapshot.MemoryOutputLimit = limit.MaxInputTokens, limit.MaxSummaryTokens
	ctx := limit.Context
	if ctx == nil {
		ctx = config.GetContext()
	}
	if limit.MaxInputTokens > 0 && TokenCountExceeds(timelineCompressionInstruction+"\n"+prompt+"\n"+timelineCompressionSchema, limit.MaxInputTokens) {
		return "", fmt.Errorf("timeline compression input exceeds safety limit %d; source preserved", limit.MaxInputTokens)
	}
	// Keep the complete output contract in the model prompt. The response
	// handler uses ActionMaker's jsonextractor without enforcing that schema.
	requestPrompt := aiprojection.CreateTag("PROMPT_SECTION", "system", timelineCompressionInstruction+"\n\n"+timelineCompressionSchema) +
		aiprojection.CreateTag("PROMPT_SECTION", "user", prompt)
	ready, completion := m.startCompressionResponse(snapshot, limit, requestPrompt)
	snapshot.MemoryCompletion = completion
	select {
	case output := <-ready:
		snapshot.Output = output
		return output.Summary, nil
	case <-completion.done:
		// A late memory error cannot reject an already usable selection.
		select {
		case output := <-ready:
			snapshot.Output = output
			return output.Summary, nil
		default:
		}
		if completion.err != nil && completion.output == nil {
			return "", completion.err
		}
		output := completion.output
		if output == nil {
			return "", fmt.Errorf("timeline compression returned no summary")
		}
		// Only summary and selection belong to the atomic Timeline commit.
		snapshot.Output = &timelineCompressionOutput{Summary: output.Summary, RetainedRange: output.RetainedRange,
			RetainedIDs: append([]int64(nil), output.RetainedIDs...)}
		return output.Summary, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
