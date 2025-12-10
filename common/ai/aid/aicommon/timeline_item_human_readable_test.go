package aicommon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestParseTimelineItemHumanReadable_NilItem(t *testing.T) {
	result := ParseTimelineItemHumanReadable(nil)
	require.Nil(t, result)
}

func TestParseTimelineItemHumanReadable_NilValue(t *testing.T) {
	item := &TimelineItem{
		deleted:   false,
		createdAt: time.Now(),
		value:     nil,
	}
	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "raw", result.Type)
}

func TestParseTimelineItemHumanReadable_TextTimelineItem_WithTaskID(t *testing.T) {
	// 模拟 re-act.go 中 AddToTimeline 生成的格式
	// [entryType] [task:taskId]:\n  content
	text := "[action] [task:task-001]:\n  This is the action content\n  with multiple lines"
	now := time.Now()

	item := &TimelineItem{
		deleted:   false,
		createdAt: now,
		value: &TextTimelineItem{
			ID:   123,
			Text: text,
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "text", result.Type)
	require.Equal(t, int64(123), result.ID)
	require.Equal(t, "action", result.EntryType)
	require.Equal(t, "task-001", result.TaskID)
	require.Equal(t, "This is the action content\nwith multiple lines", result.Content)
	require.Equal(t, text, result.RawText)
	require.Equal(t, item.createdAt.Unix(), result.Timestamp)
	require.False(t, result.Deleted)
}

func TestParseTimelineItemHumanReadable_TextTimelineItem_WithoutTaskID(t *testing.T) {
	// 没有 task 的格式: [entryType]:\n  content
	text := "[[BLUEPRINT_PROMPT_ERROR]]:\n  Simple note content"
	item := &TimelineItem{
		createdAt: time.Now(),
		value: &TextTimelineItem{
			ID:   456,
			Text: text,
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "text", result.Type)
	require.Equal(t, "[BLUEPRINT_PROMPT_ERROR]", result.EntryType)
	require.Empty(t, result.TaskID)
	require.Equal(t, "Simple note content", result.Content)
}

func TestParseTimelineItemHumanReadable_TextTimelineItem_WithBuildinEntryType(t *testing.T) {
	// 没有 task 的格式: [entryType]:\n  content
	text := "[note]:\n  Simple note content"
	item := &TimelineItem{
		createdAt: time.Now(),
		value: &TextTimelineItem{
			ID:   456,
			Text: text,
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "text", result.Type)
	require.Equal(t, "note", result.EntryType)
	require.Empty(t, result.TaskID)
	require.Equal(t, "Simple note content", result.Content)
}

func TestParseTimelineItemHumanReadable_TextTimelineItem_DefaultNote(t *testing.T) {
	// 默认 note 类型 (当 entryType 为空时)
	text := "[note] [task:task-002]:\n  Default note"
	item := &TimelineItem{
		createdAt: time.Now(),
		value: &TextTimelineItem{
			ID:   789,
			Text: text,
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "note", result.EntryType)
	require.Equal(t, "task-002", result.TaskID)
	require.Equal(t, "Default note", result.Content)
}

func TestParseTimelineItemHumanReadable_TextTimelineItem_PlainText(t *testing.T) {
	// 纯文本，没有任何格式
	text := "Just plain text without any format"
	item := &TimelineItem{
		createdAt: time.Now(),
		value: &TextTimelineItem{
			ID:   100,
			Text: text,
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "text", result.Type)
	require.Empty(t, result.EntryType)
	require.Empty(t, result.TaskID)
	require.Equal(t, text, result.Content)
}

func TestParseTimelineItemHumanReadable_TextTimelineItem_WithColonNoNewline(t *testing.T) {
	// 有冒号但没有换行
	text := "[info]: Some inline content"
	item := &TimelineItem{
		createdAt: time.Now(),
		value: &TextTimelineItem{
			ID:   101,
			Text: text,
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "info", result.EntryType)
	require.Equal(t, "Some inline content", result.Content)
}

func TestParseTimelineItemHumanReadable_UserInteraction(t *testing.T) {
	item := &TimelineItem{
		deleted:   false,
		createdAt: time.Now(),
		value: &UserInteraction{
			ID:              200,
			SystemPrompt:    "What is your name?",
			UserExtraPrompt: "My name is Test",
			Stage:           UserInteractionStage_Review,
			ShrinkResult:    "shrink result",
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "user_interaction", result.Type)
	require.Equal(t, int64(200), result.ID)
	require.Equal(t, string(UserInteractionStage_Review), result.EntryType)
	require.Equal(t, "My name is Test", result.Content)
	require.Equal(t, "shrink result", result.ShrinkResult)
}

func TestParseTimelineItemHumanReadable_ToolResult(t *testing.T) {
	item := &TimelineItem{
		deleted:   false,
		createdAt: time.Now(),
		value: &aitool.ToolResult{
			ID:          300,
			Name:        "test_tool",
			Description: "A test tool",
			Param:       map[string]any{"key": "value"},
			Success:     true,
			Data:        "tool output data",
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "tool_result", result.Type)
	require.Equal(t, int64(300), result.ID)
	require.NotEmpty(t, result.Content)
}

func TestParseTimelineItemHumanReadable_DeletedItem(t *testing.T) {
	item := &TimelineItem{
		deleted:   true,
		createdAt: time.Now(),
		value: &TextTimelineItem{
			ID:   400,
			Text: "[deleted]:\n  Some deleted content",
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.True(t, result.Deleted)
}

func TestParseTimelineItemHumanReadable_WithShrinkResult(t *testing.T) {
	item := &TimelineItem{
		createdAt: time.Now(),
		value: &TextTimelineItem{
			ID:                  500,
			Text:                "[action]:\n  Content",
			ShrinkResult:        "shrink result",
			ShrinkSimilarResult: "similar result",
		},
	}

	result := ParseTimelineItemHumanReadable(item)
	require.NotNil(t, result)
	require.Equal(t, "shrink result", result.ShrinkResult)
	require.Equal(t, "similar result", result.ShrinkSimilarResult)
}

func TestParseTimelineItemsHumanReadable_Nil(t *testing.T) {
	result := ParseTimelineItemsHumanReadable(nil)
	require.Nil(t, result)
}

func TestParseTimelineItemsHumanReadable_Empty(t *testing.T) {
	result := ParseTimelineItemsHumanReadable([]*TimelineItem{})
	require.NotNil(t, result)
	require.Empty(t, result)
}

func TestParseTimelineItemsHumanReadable_Multiple(t *testing.T) {
	items := []*TimelineItem{
		{
			createdAt: time.Now(),
			value: &TextTimelineItem{
				ID:   1,
				Text: "[note]:\n  Note 1",
			},
		},
		{
			createdAt: time.Now(),
			value: &TextTimelineItem{
				ID:   2,
				Text: "[action] [task:t1]:\n  Action 1",
			},
		},
		{
			createdAt: time.Now(),
			value: &UserInteraction{
				ID:              3,
				Stage:           UserInteractionStage_FreeInput,
				UserExtraPrompt: "User input",
			},
		},
	}

	results := ParseTimelineItemsHumanReadable(items)
	require.Len(t, results, 3)

	require.Equal(t, "text", results[0].Type)
	require.Equal(t, "note", results[0].EntryType)

	require.Equal(t, "text", results[1].Type)
	require.Equal(t, "action", results[1].EntryType)
	require.Equal(t, "t1", results[1].TaskID)

	require.Equal(t, "user_interaction", results[2].Type)
	require.Equal(t, string(UserInteractionStage_FreeInput), results[2].EntryType)
}

func TestRemoveIndent(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		prefix   string
		expected string
	}{
		{
			name:     "simple indent",
			text:     "  line1\n  line2",
			prefix:   "  ",
			expected: "line1\nline2",
		},
		{
			name:     "no indent",
			text:     "line1\nline2",
			prefix:   "  ",
			expected: "line1\nline2",
		},
		{
			name:     "partial indent",
			text:     "  line1\nline2\n  line3",
			prefix:   "  ",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "empty string",
			text:     "",
			prefix:   "  ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeIndent(tt.text, tt.prefix)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseTextTimelineItem_EmptyText(t *testing.T) {
	result := &TimelineItemHumanReadable{}
	parseTextTimelineItem(result, "")
	require.Empty(t, result.EntryType)
	require.Empty(t, result.TaskID)
	require.Empty(t, result.Content)
}

func TestParseTextTimelineItem_ComplexTaskID(t *testing.T) {
	// 测试复杂的任务 ID
	text := "[action] [task:plan-1-subtask-2-step-3]:\n  Complex task"
	result := &TimelineItemHumanReadable{}
	parseTextTimelineItem(result, text)

	require.Equal(t, "action", result.EntryType)
	require.Equal(t, "plan-1-subtask-2-step-3", result.TaskID)
	require.Equal(t, "Complex task", result.Content)
}

func TestParseTextTimelineItem_MultilineContent(t *testing.T) {
	text := "[result] [task:t1]:\n  Line 1\n  Line 2\n  Line 3"
	result := &TimelineItemHumanReadable{}
	parseTextTimelineItem(result, text)

	require.Equal(t, "result", result.EntryType)
	require.Equal(t, "t1", result.TaskID)
	require.Equal(t, "Line 1\nLine 2\nLine 3", result.Content)
}

func TestParseTextTimelineItem_WithSpecialEntryType(t *testing.T) {
	text := "[current task user input]:\n  Special entry type"
	result := &TimelineItemHumanReadable{}
	parseTextTimelineItem(result, text)

	require.Equal(t, "current task user input", result.EntryType)
	require.Equal(t, "Special entry type", result.Content)
	require.Equal(t, "user_input", result.Type)
}
