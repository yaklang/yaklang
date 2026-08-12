package aicommon

import (
	"strings"
)

// projectTimelineItemForPrompt removes control-plane bookkeeping that is
// already represented by materialized prompt state. It never mutates the raw
// Timeline item: UI, diff, persistence, fork and rollback continue to observe
// the original event stream.
func projectTimelineItemForPrompt(item *TimelineItem) *TimelineItem {
	if item == nil || item.deleted || item.value == nil {
		return nil
	}
	textItem, ok := item.value.(*TextTimelineItem)
	if !ok || textItem == nil {
		// ToolResult and UserInteraction are deliberately opaque to this pass.
		return item
	}

	// 直接从原始文本提取 entryType, 不依赖 parseTextTimelineItem
	// (parseTextTimelineItem 已不再做正则匹配设置 EntryType).
	entryType := extractTextEntryType(textItem.Text)
	category := normalizeTimelinePromptCategory(entryType)
	switch category {
	case "TODO_DELTA", "EVIDENCE_OPS", "MODEL_THINKING":
		return nil
	case "ITERATION":
		return item
	case "TODO_DELTA_ERROR":
		content := extractTextTimelineContent(textItem.Text)
		filtered := filterRedundantTodoErrorLines(content)
		if filtered == "" {
			return nil
		}
		if filtered == strings.TrimSpace(content) {
			return item
		}
		return cloneTextTimelineItemForPrompt(item, textItem, replaceTimelineTextBody(textItem.Text, filtered))
	default:
		return item
	}
}

func normalizeTimelinePromptCategory(category string) string {
	return strings.ToUpper(strings.Trim(strings.TrimSpace(category), "[] \t\r\n"))
}

func filterRedundantTodoErrorLines(content string) string {
	kept := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "redundant ") && strings.Contains(lower, "todo already ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func replaceTimelineTextBody(text, body string) string {
	if idx := strings.Index(text, ":\n"); idx >= 0 {
		return text[:idx+2] + body
	}
	if idx := strings.Index(text, ":"); idx >= 0 {
		return text[:idx+1] + "\n" + body
	}
	return body
}

func cloneTextTimelineItemForPrompt(item *TimelineItem, textItem *TextTimelineItem, text string) *TimelineItem {
	textCopy := *textItem
	textCopy.Text = text
	// A precomputed shrink belongs to the original text and must not bypass the
	// prompt projection's filtered body.
	textCopy.ShrinkResult = ""
	textCopy.ShrinkSimilarResult = ""
	return &TimelineItem{createdAt: item.createdAt, value: &textCopy}
}

func projectTimelineItemsForPrompt(items []*TimelineItem) []*TimelineItem {
	projected := make([]*TimelineItem, 0, len(items))
	for _, item := range items {
		if promptItem := projectTimelineItemForPrompt(item); promptItem != nil {
			projected = append(projected, promptItem)
		}
	}
	return projected
}

// projectTimelineRenderableBlocksForPrompt preserves the raw block topology
// and stable nonces. Empty projected interval blocks remain present so noise
// filtering cannot move the Frozen/Open boundary.
func projectTimelineRenderableBlocksForPrompt(blocks TimelineRenderableBlocks) TimelineRenderableBlocks {
	projected := make(TimelineRenderableBlocks, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case *TimelineIntervalBlock:
			if typed == nil {
				continue
			}
			copyBlock := *typed
			copyBlock.Items = projectTimelineItemsForPrompt(typed.Items)
			projected = append(projected, &copyBlock)
		default:
			// Existing compressed heads are historical facts. Rewriting them here
			// would alter reducer semantics and is outside projection cleanup.
			if block != nil {
				projected = append(projected, block)
			}
		}
	}
	return projected
}
