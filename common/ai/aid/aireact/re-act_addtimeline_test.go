package aireact

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestReAct_AddToTimeline_NoIndentInjected 验证 AddToTimeline 写入 Timeline 时
// 不再给 body 注入 "  " 前缀 (源头 dedent), 节省 prompt token.
//
// 关键词: AddToTimeline 无缩进, timeline render dedent, prompt token 节省
func TestReAct_AddToTimeline_NoIndentInjected(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			time.Sleep(time.Second)
			return nil, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)
	require.NotNil(t, ins)

	// 调 AddToTimeline 注入一条多行 body. 修复前 body 整体会被 PrefixLines("  ")
	// 处理一遍, 修复后 body 顶头, 只有 header line "[ENTRY]:\n" 在前.
	body := "line1\nline2\nline3"
	ins.AddToTimeline("ENTRY", body)

	// 从 Timeline 取出最后一条 item, 断言 *TextTimelineItem.Text 形如:
	//   "[ENTRY]:\nline1\nline2\nline3"  (无 task)
	// 而不是修复前的:
	//   "[ENTRY]:\n  line1\n  line2\n  line3"
	tl := ins.config.Timeline
	require.NotNil(t, tl)
	idMap := tl.GetIdToTimelineItem()
	require.NotNil(t, idMap)
	require.Greater(t, idMap.Len(), 0, "timeline should contain at least one item")

	keys := idMap.Keys()
	lastKey := keys[len(keys)-1]
	item, ok := idMap.Get(lastKey)
	require.True(t, ok)
	require.NotNil(t, item)

	value := item.GetValue()
	textItem, ok := value.(*aicommon.TextTimelineItem)
	require.Truef(t, ok, "last timeline value should be *TextTimelineItem, got %T", value)

	text := textItem.Text
	t.Logf("timeline text raw: %q", text)

	// 必须能匹配 header
	require.True(t, strings.HasPrefix(text, "[ENTRY]"),
		"text should start with header [ENTRY], got: %q", text)
	require.Contains(t, text, ":\n", "text should contain ':\\n' header separator")

	// body 顶头, 无 "  " 前缀
	require.Contains(t, text, "\nline1\n",
		"body line1 should sit flush left (no '  ' prefix), got: %q", text)
	require.Contains(t, text, "\nline2\n",
		"body line2 should sit flush left, got: %q", text)
	require.True(t, strings.HasSuffix(text, "\nline3") || strings.HasSuffix(text, "\nline3\n"),
		"body line3 should sit flush left at tail, got: %q", text)

	// 反向断言: 不应再出现 "  line1" / "  line2" / "  line3" 这种带 2 空格前缀的行
	require.NotContains(t, text, "\n  line1",
		"body line1 should NOT carry the historical '  ' prefix, got: %q", text)
	require.NotContains(t, text, "\n  line2",
		"body line2 should NOT carry the historical '  ' prefix, got: %q", text)
	require.NotContains(t, text, "\n  line3",
		"body line3 should NOT carry the historical '  ' prefix, got: %q", text)
}
