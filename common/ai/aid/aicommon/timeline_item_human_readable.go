package aicommon

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TimelineItemType 是 TimelineItem EntryType 的类型
const (
	TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT = "current task user input"
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

var EntryTypeToTimelineItemType = map[string]string{
	TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT: "user_input",
}

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
//
// 当前格式 (re-act.go:AddToTimeline 修复后): [entryType] [task:taskId]:\nbody
//
//	或 [entryType]:\nbody
//
// 历史格式 (修复前): [entryType] [task:taskId]:\n  body
// 后者 body 整体多缩 2 空格, 是早期 utils.PrefixLines(content, "  ") 注入的
// "为人类阅读 dump 而打的视觉嵌套". 现在 timeline render 已经为每条 item 单
// 独输出 'HH:MM:SS [type/...]' 行头, 缩进对 LLM 不再有信息量, 已从源头去掉.
//
// 解析时统一调 removeIndent 把"可能存在的"两空格前缀消掉, 让两种格式产出
// 一致 Content; 新数据没有前缀, removeIndent 退化成 no-op (TrimPrefix 找不
// 到时返回原行), 安全幂等, 兼容历史持久化的 timeline 回放.
//
// 关键词: parseTextTimelineItem 历史兼容, removeIndent 安全 no-op
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
		// 兼容历史持久化 timeline: 旧格式 body 整体多缩 "  ", 这里消除回去;
		// 新格式 (修复后) body 顶头无前缀, removeIndent 是 no-op, 不影响.
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

	if itemType, ok := EntryTypeToTimelineItemType[result.EntryType]; ok {
		result.Type = itemType
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
