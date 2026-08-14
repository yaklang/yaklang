package aicommon

import (
	"strings"
)

// projectTimelineItemForPrompt removes control-plane bookkeeping that is
// already represented by materialized prompt state. It never mutates the raw
// Timeline item: UI, diff, persistence, fork and rollback continue to observe
// the original event stream.
func projectTimelineItemForPrompt(item *TimelineItem) *TimelineItem {
	return projectTimelineItemForPromptWithModelReplay(item, false)
}

func projectTimelineItemForPromptWithModelReplay(item *TimelineItem, allowModelReplay bool) *TimelineItem {
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
	case "TODO_DELTA", "EVIDENCE_OPS":
		return nil
	case "MODEL_THINKING":
		if !allowModelReplay || strings.TrimSpace(textItem.PromptText) == "" {
			return nil
		}
		return cloneTextTimelineItemForPrompt(item, textItem, textItem.PromptText)
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
	return projectTimelineItemsForPromptWithLatestModelReplay(items, 0)
}

func projectTimelineItemsForPromptWithLatestModelReplay(items []*TimelineItem, latestReplayID int64) []*TimelineItem {
	projected := make([]*TimelineItem, 0, len(items))
	for _, item := range items {
		allowReplay := latestReplayID > 0 && item != nil && item.GetID() == latestReplayID
		if promptItem := projectTimelineItemForPromptWithModelReplay(item, allowReplay); promptItem != nil {
			projected = append(projected, promptItem)
		}
	}
	return projected
}

// projectTimelineRenderableBlocksForPrompt preserves the raw block topology
// and stable nonces. Empty projected interval blocks remain present so noise
// filtering cannot move the Frozen/Open boundary.
func projectTimelineRenderableBlocksForPrompt(blocks TimelineRenderableBlocks) TimelineRenderableBlocks {
	return projectTimelineRenderableBlocksForPromptWithModelReplay(blocks, false)
}

// projectTimelineRenderableBlocksForPromptWithLatestModelReplay preserves the
// existing bucket topology while allowing exactly the newest successful model
// replay record into the prompt. Older model thinking remains UI-only.
func projectTimelineRenderableBlocksForPromptWithLatestModelReplay(blocks TimelineRenderableBlocks) TimelineRenderableBlocks {
	return projectTimelineRenderableBlocksForPromptWithModelReplay(blocks, true)
}

func projectTimelineRenderableBlocksForPromptWithModelReplay(blocks TimelineRenderableBlocks, includeLatestReplay bool) TimelineRenderableBlocks {
	var latestReplayID int64
	if includeLatestReplay {
		for _, block := range blocks {
			interval, ok := block.(*TimelineIntervalBlock)
			if !ok || interval == nil {
				continue
			}
			for _, item := range interval.Items {
				textItem, ok := timelineTextItem(item)
				if !ok || strings.TrimSpace(textItem.PromptText) == "" || normalizeTimelinePromptCategory(extractTextEntryType(textItem.Text)) != "MODEL_THINKING" {
					continue
				}
				if id := item.GetID(); id > latestReplayID {
					latestReplayID = id
				}
			}
		}
	}
	projected := make(TimelineRenderableBlocks, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case *TimelineIntervalBlock:
			if typed == nil {
				continue
			}
			copyBlock := *typed
			copyBlock.Items = projectTimelineItemsForPromptWithLatestModelReplay(typed.Items, latestReplayID)
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

func timelineTextItem(item *TimelineItem) (*TextTimelineItem, bool) {
	if item == nil || item.deleted || item.value == nil {
		return nil, false
	}
	textItem, ok := item.value.(*TextTimelineItem)
	return textItem, ok && textItem != nil
}
