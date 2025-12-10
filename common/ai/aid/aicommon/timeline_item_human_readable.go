package aicommon

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TimelineItemHumanReadable 是 TimelineItem 的人类可读版本
// 它解析 TimelineItem 中的 value 字段，提取出有意义的信息
type TimelineItemHumanReadable struct {
	// 原始 TimelineItem 属性
	Deleted   bool   `json:"deleted"`
	Timestamp int64  `json:"timestamp"`
	ID        int64  `json:"id"`
	Type      string `json:"type"` // "tool_result", "user_interaction", "text", "raw"

	// 解析出的字段（针对 TextTimelineItem）
	EntryType string `json:"entry_type,omitempty"` // 如 "note", "action" 等
	TaskID    string `json:"task_id,omitempty"`    // 任务ID
	Content   string `json:"content"`              // 实际内容

	// 原始文本（用于调试）
	RawText string `json:"raw_text,omitempty"`

	// 收缩结果
	ShrinkResult        string `json:"shrink_result,omitempty"`
	ShrinkSimilarResult string `json:"shrink_similar_result,omitempty"`
}

// 正则表达式用于解析 TextTimelineItem 的 Text 字段
// 格式: [entryType] [task:taskId]:\n  content
// 或:   [entryType]:\n  content
var (
	// 匹配有 task 的格式: [entryType] [task:taskId]:
	withTaskRegex = regexp.MustCompile(`^\[(.+?)\] \[task:([^\]]+)\]:`)
	// 匹配没有 task 的格式: [entryType]:
	withoutTaskRegex = regexp.MustCompile(`^\[(.+?)\]:`)
)

// ParseTimelineItemHumanReadable 解析 TimelineItem 对象生成 TimelineItemHumanReadable 对象
func ParseTimelineItemHumanReadable(item *TimelineItem) *TimelineItemHumanReadable {
	if item == nil {
		return nil
	}

	result := &TimelineItemHumanReadable{
		Deleted:   item.deleted,
		Timestamp: item.createdAt.Unix(),
		ID:        item.GetID(),
	}

	if item.value == nil {
		result.Type = "raw"
		return result
	}

	// 根据 value 类型进行解析
	switch v := item.value.(type) {
	case *aitool.ToolResult:
		result.Type = "tool_result"
		result.Content = v.String()
		result.ShrinkResult = v.GetShrinkResult()
		result.ShrinkSimilarResult = v.GetShrinkSimilarResult()

	case *UserInteraction:
		result.Type = "user_interaction"
		result.EntryType = string(v.Stage)
		result.Content = v.UserExtraPrompt
		result.ShrinkResult = v.GetShrinkResult()
		result.ShrinkSimilarResult = v.GetShrinkSimilarResult()

	case *TextTimelineItem:
		result.Type = "text"
		result.RawText = v.Text
		result.ShrinkResult = v.GetShrinkResult()
		result.ShrinkSimilarResult = v.GetShrinkSimilarResult()

		// 解析 TextTimelineItem 的 Text 字段
		parseTextTimelineItem(result, v.Text)

	default:
		result.Type = "raw"
		result.Content = item.String()
	}

	return result
}

// parseTextTimelineItem 解析 TextTimelineItem 的 Text 字段
// 格式: [entryType] [task:taskId]:\n  content
// 或:   [entryType]:\n  content
func parseTextTimelineItem(result *TimelineItemHumanReadable, text string) {
	if text == "" {
		return
	}

	// 优先匹配有 task 的格式: [entryType] [task:taskId]:
	if matches := withTaskRegex.FindStringSubmatch(text); len(matches) > 2 {
		result.EntryType = matches[1]
		result.TaskID = matches[2]
	} else if matches := withoutTaskRegex.FindStringSubmatch(text); len(matches) > 1 {
		// 匹配没有 task 的格式: [entryType]:
		result.EntryType = matches[1]
	}

	// 解析内容：找到第一个 ":\n" 之后的内容
	colonIndex := strings.Index(text, ":\n")
	if colonIndex != -1 {
		content := text[colonIndex+2:] // 跳过 ":\n"
		// 移除每行开头的两个空格缩进 (utils.PrefixLines 添加的 "  ")
		result.Content = removeIndent(content, "  ")
	} else {
		// 如果没有找到 ":\n"，尝试找 ":" 后的内容
		colonIndex = strings.Index(text, ":")
		if colonIndex != -1 {
			result.Content = strings.TrimSpace(text[colonIndex+1:])
		} else {
			// 都没有找到，整个文本就是内容
			result.Content = text
		}
	}
}

// removeIndent 移除每行开头的指定前缀
func removeIndent(text string, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, prefix)
	}
	return strings.Join(lines, "\n")
}

// ParseTimelineItemsHumanReadable 批量解析 TimelineItem 列表
func ParseTimelineItemsHumanReadable(items []*TimelineItem) []*TimelineItemHumanReadable {
	if items == nil {
		return nil
	}

	results := make([]*TimelineItemHumanReadable, 0, len(items))
	for _, item := range items {
		if parsed := ParseTimelineItemHumanReadable(item); parsed != nil {
			results = append(results, parsed)
		}
	}
	return results
}
