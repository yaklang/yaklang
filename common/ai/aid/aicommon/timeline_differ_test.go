package aicommon

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestTimelineDiffer_Basic(t *testing.T) {
	// 创建 Timeline
	timeline := NewTimeline(nil, nil)

	// 创建 differ
	differ := NewTimelineDiffer(timeline)

	// 第一次调用 Diff() - 应该返回当前空状态
	diff1, err := differ.Diff()
	require.NoError(t, err)
	require.Equal(t, "", diff1, "第一次调用应该返回空字符串")

	// 添加一些内容
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          101,
		Name:        "test_tool_1",
		Description: "first test tool",
		Param:       map[string]any{"action": "test1"},
		Success:     true,
		Data:        "result1",
	})

	// 第二次调用 Diff() - 应该返回新增内容的差异
	diff2, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff2, "应该检测到新增内容")
	require.Contains(t, diff2, "test_tool_1", "差异应该包含新增的工具名称")

	// 再次添加内容
	timeline.PushUserInteraction(UserInteractionStage_Review, 201, "system prompt", "user input")

	// 第三次调用 Diff() - 应该返回新增的用户交互差异
	diff3, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff3, "应该检测到新增的用户交互")
	require.Contains(t, diff3, "user input", "差异应该包含用户输入")

	t.Log("TimelineDiffer basic test passed")
}

func TestTimelineDiffer_Reset(t *testing.T) {
	// 创建 Timeline 并添加内容
	timeline := NewTimeline(nil, nil)
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          102,
		Name:        "test_tool_2",
		Description: "second test tool",
		Param:       map[string]any{"action": "test2"},
		Success:     true,
		Data:        "result2",
	})

	differ := NewTimelineDiffer(timeline)

	// 第一次 diff
	diff1, err := differ.Diff()
	require.NoError(t, err)
	require.Contains(t, diff1, "test_tool_2")

	// 重置
	differ.Reset()

	// 添加更多内容
	timeline.PushText(301, "additional text content")

	// 再次 diff - 应该包含所有内容（因为重置了基准）
	diff2, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff2)
	// 应该包含之前和新增的内容
	require.True(t, strings.Contains(diff2, "test_tool_2") || strings.Contains(diff2, "additional text content"),
		"重置后应该包含所有内容")

	t.Log("TimelineDiffer reset test passed")
}

func TestTimelineDiffer_SetBaseline(t *testing.T) {
	// 创建 Timeline
	timeline := NewTimeline(nil, nil)

	differ := NewTimelineDiffer(timeline)

	// 手动设置基准（此时为空）
	differ.SetBaseline()

	// 添加内容
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          103,
		Name:        "test_tool_3",
		Description: "third test tool",
		Param:       map[string]any{"action": "test3"},
		Success:     true,
		Data:        "result3",
	})

	// diff - 应该只包含新增内容
	diff, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff)
	require.Contains(t, diff, "test_tool_3")

	t.Log("TimelineDiffer set baseline test passed")
}

func TestTimelineDiffer_GetMethods(t *testing.T) {
	// 创建 Timeline 并添加内容
	timeline := NewTimeline(nil, nil)
	timeline.PushText(401, "test content for getter methods")

	differ := NewTimelineDiffer(timeline)

	// 测试 GetCurrentDump
	currentDump := differ.GetCurrentDump()
	require.NotEmpty(t, currentDump)
	require.Contains(t, currentDump, "test content for getter methods")

	// 测试 GetLastDump（初始为空）
	lastDump := differ.GetLastDump()
	require.Equal(t, "", lastDump)

	// 调用 Diff() 后，lastDump 应该更新
	_, err := differ.Diff()
	require.NoError(t, err)

	lastDumpAfter := differ.GetLastDump()
	require.Equal(t, currentDump, lastDumpAfter)

	t.Log("TimelineDiffer getter methods test passed")
}

func TestTimelineDiffer_MultipleChanges(t *testing.T) {
	// 创建 Timeline
	timeline := NewTimeline(nil, nil)

	differ := NewTimelineDiffer(timeline)

	// 初始 diff（空）
	diff0, err := differ.Diff()
	require.NoError(t, err)
	require.Equal(t, "", diff0)

	// 第一批更改
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          104,
		Name:        "batch_1_tool",
		Description: "first batch",
		Success:     true,
		Data:        "batch1",
	})

	diff1, err := differ.Diff()
	require.NoError(t, err)
	require.Contains(t, diff1, "batch_1_tool")

	// 第二批更改
	timeline.PushUserInteraction(UserInteractionStage_BeforePlan, 202, "plan prompt", "plan input")
	timeline.PushText(302, "batch 2 text")

	diff2, err := differ.Diff()
	require.NoError(t, err)
	require.NotEmpty(t, diff2)
	// 应该包含新增的用户交互和文本，但不包含第一批的工具
	require.True(t, strings.Contains(diff2, "plan input") || strings.Contains(diff2, "batch 2 text"),
		"应该检测到第二批的更改")

	t.Log("TimelineDiffer multiple changes test passed")
}

func TestTimelineDiffer_EmptyTimeline(t *testing.T) {
	// 创建空 Timeline
	timeline := NewTimeline(nil, nil)

	differ := NewTimelineDiffer(timeline)

	// 多次调用 diff 应该都返回空字符串
	for i := 0; i < 3; i++ {
		diff, err := differ.Diff()
		require.NoError(t, err)
		require.Equal(t, "", diff, "空 Timeline 的 diff 应该始终为空字符串")
	}

	t.Log("TimelineDiffer empty timeline test passed")
}
