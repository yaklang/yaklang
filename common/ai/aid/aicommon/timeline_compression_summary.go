package aicommon

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
)

var timelineCompressionSchema = promptloader.MustLoad("ai/aid/aicommon/prompts/timeline/compression.json")

var compressionPromptTemplate = template.Must(template.New("timeline-compression").Parse(promptloader.MustLoad("ai/aid/aicommon/prompts/timeline/compression.txt")))

type compressionPromptData struct {
	Capacity string
	Timeline string
	Schema   string
}

// renderCompressionSummaryPrompt builds one reduction request from three parts:
// the fixed instruction, a computed capacity section, and rendered Timeline
// content. It never freezes, deletes, or commits anything.
func renderCompressionSummaryPrompt(snapshot *timelineCompressionSnapshot) (string, error) {
	if snapshot == nil {
		return "", fmt.Errorf("invalid timeline compression snapshot")
	}
	previous := ""
	if snapshot.Head != nil {
		previous = snapshot.Head.Text
	}
	if strings.TrimSpace(previous) == "" && strings.TrimSpace(renderCompressionSnapshotItems(snapshot.Items)) == "" &&
		!snapshot.hasPromotedItems() &&
		(!snapshot.FinalizingMemory || (len(snapshot.UserContexts) == 0 && strings.TrimSpace(snapshot.Evidence) == "")) {
		return "", fmt.Errorf("timeline compression has no history to summarize")
	}
	var buf bytes.Buffer
	if err := compressionPromptTemplate.Execute(&buf, compressionPromptData{
		Capacity: renderCompressionCapacity(snapshot),
		Timeline: renderCompressionTimeline(snapshot),
		Schema:   timelineCompressionSchema,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderCompressionCapacity(snapshot *timelineCompressionSnapshot) string {
	budget := snapshot.PromptBudget
	target := snapshot.TargetTokens
	if target == 0 {
		target = budget.Target
	}
	ordinary, promoted := compressionCapacityParts(budget)
	var b strings.Builder
	b.WriteString("# 容量\n")
	if budget.Current > 0 {
		fmt.Fprintf(&b, "当前主模型 Timeline：%d tokens。\n", budget.Current)
	}
	if target > 0 {
		fmt.Fprintf(&b, "目标：不超过 %d tokens。目标包含 summary、保留原文、仍保留的 promoted 正文和不可删除的用户输入。\n", target)
	}
	if budget.Current > 0 && target > 0 && budget.Current > target {
		fmt.Fprintf(&b, "还需要减少：%d tokens。按这个缺口估算效果。\n", budget.Current-target)
	}
	if ordinary > 0 || promoted > 0 {
		fmt.Fprintf(&b, "两个可优化部分：普通历史 %d tokens，promoted 内容 %d tokens。\n", ordinary, promoted)
		b.WriteString("普通历史通过 summary 和 ratain_timeline_item_range 压缩。promoted 内容通过删除不再需要的 RETIRE_ID 压缩；用户输入不可删除。\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func compressionCapacityParts(budget timelineCompressionPromptBudget) (ordinary, promoted int64) {
	for _, section := range budget.Sections {
		switch section.Name {
		case "frozen_timeline", "open_timeline":
			ordinary += section.Tokens
		case "recent_tools", "skill_content", "evidence":
			promoted += section.Tokens
		}
	}
	return ordinary, promoted
}

func renderCompressionTimeline(snapshot *timelineCompressionSnapshot) string {
	var b strings.Builder
	b.WriteString("# Timeline\n")
	previous := ""
	if snapshot.Head != nil {
		previous = snapshot.Head.Text
	}
	writeCompressionPart(&b, "旧历史摘要", previous)
	items := append([]timelineCompressionSnapshotItem(nil), snapshot.Items...)
	for _, promoted := range snapshot.PromotedItems {
		item := promoted.timelineCompressionSnapshotItem
		item.PromptText = renderCompressionPromotedItem(promoted)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writeCompressionPart(&b, "历史条目", renderCompressionSnapshotItems(items))
	writeCompressionPart(&b, "当前用户要求", compressionRetainedText(snapshot, "user_query"))
	writeCompressionPart(&b, "用户输入", compressionUserText(snapshot))
	writeCompressionPart(&b, "TODO", compressionRetainedText(snapshot, "todo"))
	writeCompressionPart(&b, "其他保留上下文", compressionExtraContext(snapshot))
	writeCompressionPart(&b, "本 session 的记忆候选", renderSessionMemoryCandidates(snapshot.SessionMemoryCandidates))
	if !snapshot.hasEvidenceItems() {
		writeCompressionPart(&b, "旧会话证据", snapshot.Evidence)
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeCompressionPart(b *strings.Builder, title, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	fmt.Fprintf(b, "## %s\n%s\n\n", title, content)
}

func compressionRetainedText(snapshot *timelineCompressionSnapshot, key string) string {
	if snapshot.RetainedContext == nil {
		return ""
	}
	return snapshot.RetainedContext[key]
}

func compressionUserText(snapshot *timelineCompressionSnapshot) string {
	var b strings.Builder
	retained := compressionRetainedText(snapshot, "frozen_user_context")
	if retained != "" {
		b.WriteString(retained)
		b.WriteByte('\n')
	}
	for _, entry := range snapshot.UserContexts {
		// The retained block already carries frozen user context.
		if retained != "" && entry.Frozen {
			continue
		}
		fmt.Fprintf(&b, "USER_INPUT_ID=%d 不可删除\n%s\n", entry.ID, entry.Text)
	}
	return b.String()
}

func compressionExtraContext(snapshot *timelineCompressionSnapshot) string {
	known := map[string]bool{"user_query": true, "frozen_user_context": true, "todo": true, "task_instruction": true}
	var keys []string
	for key := range snapshot.RetainedContext {
		if !known[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s:\n%s\n", key, snapshot.RetainedContext[key])
	}
	return b.String()
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
	if limit.MaxInputTokens > 0 && TokenCountExceeds(prompt, limit.MaxInputTokens) {
		return "", fmt.Errorf("timeline compression input exceeds safety limit %d; source preserved", limit.MaxInputTokens)
	}
	requestPrompt := prompt
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
			RetainedIDs:            append([]int64(nil), output.RetainedIDs...),
			RetiredPromotedEntries: append([]int64(nil), output.RetiredPromotedEntries...)}
		return output.Summary, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
