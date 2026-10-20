package aicommon

import (
	"fmt"
	"strings"
)

func compressionRetainedText(snapshot *timelineCompressionSnapshot, key string) string {
	if snapshot == nil || snapshot.RetainedContext == nil {
		return ""
	}
	return snapshot.RetainedContext[key]
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
