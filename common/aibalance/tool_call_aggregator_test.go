package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// tool_call_aggregator_test.go - ToolCallAggregator 单元测试.
//
// 覆盖:
//   - env AIBALANCE_TOOL_CALL_AGG=off → NewToolCallAggregator 返回 nil, 全程 no-op
//   - 单 tool_call 25 帧 incremental delta (复现用户 trace 抓到的 wHxEjDxC 场景),
//     首帧带 name+id, 后续 24 帧只带 arguments 增量, Flush 后聚合出完整 args
//   - 并行多 tool_call: 新 index 出现时上一个 index 立刻 flush
//   - Flush 幂等: 重复 Flush 不会重复触发日志/状态
//   - nil-safe receiver: 所有方法对 nil aggregator 是 no-op
//
// 关键词: ToolCallAggregator unit test, incremental 聚合验证, 复现 wHxEjDxC

func TestToolCallAgg_EnvDisabled_ReturnsNil(t *testing.T) {
	t.Setenv("AIBALANCE_TOOL_CALL_AGG", "off")
	require.False(t, ToolCallAggregatorEnabled())
	a := NewToolCallAggregator("test")
	assert.Nil(t, a, "env off: NewToolCallAggregator 必须返回 nil")

	// nil-safe receiver
	a.Observe([]*aispec.ToolCall{{Index: 0, Function: aispec.FuncReturn{Name: "x"}}})
	a.Flush()
	assert.Nil(t, a.Snapshot())
}

func TestToolCallAgg_EnvDefaults_Enabled(t *testing.T) {
	t.Setenv("AIBALANCE_TOOL_CALL_AGG", "")
	require.True(t, ToolCallAggregatorEnabled(), "default: enabled")
	a := NewToolCallAggregator("default-on")
	require.NotNil(t, a)
}

func TestToolCallAgg_EnvVariants(t *testing.T) {
	cases := map[string]bool{
		"":         true,
		"on":       true,
		"1":        true,
		"true":     true,
		"yes":      true,
		"off":      false,
		"0":        false,
		"false":    false,
		"no":       false,
		"disable":  false,
		"disabled": false,
	}
	for v, want := range cases {
		t.Run("env="+v, func(t *testing.T) {
			t.Setenv("AIBALANCE_TOOL_CALL_AGG", v)
			assert.Equal(t, want, ToolCallAggregatorEnabled(), "env %q expect enabled=%v", v, want)
		})
	}
}

func TestToolCallAgg_Single_25Fragments_FlushedComplete(t *testing.T) {
	t.Setenv("AIBALANCE_TOOL_CALL_AGG", "on")
	a := NewToolCallAggregator("single-frag")
	require.NotNil(t, a)

	// 第 1 帧: name + id + type, 无 args
	a.Observe([]*aispec.ToolCall{{
		Index: 0,
		ID:    "call_00_SlqvUZe7HQvBmxKKBZQ00401",
		Type:  "function",
		Function: aispec.FuncReturn{
			Name: "webfetch",
		},
	}})

	// 第 2~25 帧: 只携带 args 增量 (复现 wHxEjDxC 真实切割)
	fragments := []string{
		"{", "\"", "url", "\"", ": ", "\"",
		"https", "://", "id", ".red", "h", "aze", ".top", "/",
		"\"", ", ", "\"", "format", "\"", ": ", "\"", "text", "\"", "}",
	}
	for _, f := range fragments {
		a.Observe([]*aispec.ToolCall{{
			Index: 0,
			Function: aispec.FuncReturn{
				Arguments: f,
			},
		}})
	}

	// 还没 Flush, snapshot 应该已经累积出完整内容 (Flushed=false)
	snap := a.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, 0, snap[0].Index)
	assert.Equal(t, "webfetch", snap[0].Name)
	assert.Equal(t, "call_00_SlqvUZe7HQvBmxKKBZQ00401", snap[0].ID)
	assert.Equal(t, "function", snap[0].TypeName)
	assert.Equal(t, `{"url": "https://id.redhaze.top/", "format": "text"}`, snap[0].Args,
		"25 帧增量 args 必须无损拼接 (含分隔空格, 上游原样)")
	assert.False(t, snap[0].Flushed)

	a.Flush()
	snap2 := a.Snapshot()
	assert.True(t, snap2[0].Flushed, "Flush 后 Flushed 必须为 true")
}

func TestToolCallAgg_ParallelTwoIndices_NewIndexTriggersFlushPrev(t *testing.T) {
	t.Setenv("AIBALANCE_TOOL_CALL_AGG", "on")
	a := NewToolCallAggregator("parallel")
	require.NotNil(t, a)

	// 第 1 个 tool_call (index=0)
	a.Observe([]*aispec.ToolCall{{
		Index:    0,
		ID:       "call_A",
		Type:     "function",
		Function: aispec.FuncReturn{Name: "bash"},
	}})
	a.Observe([]*aispec.ToolCall{{
		Index:    0,
		Function: aispec.FuncReturn{Arguments: `{"cmd":"ls"}`},
	}})

	// 第 2 个 tool_call (index=1) 出现 → 应该 flush 掉 index=0
	a.Observe([]*aispec.ToolCall{{
		Index:    1,
		ID:       "call_B",
		Type:     "function",
		Function: aispec.FuncReturn{Name: "webfetch"},
	}})
	a.Observe([]*aispec.ToolCall{{
		Index:    1,
		Function: aispec.FuncReturn{Arguments: `{"url":"https://x"}`},
	}})

	snap := a.Snapshot()
	require.Len(t, snap, 2, "并行 2 个 index 必须各自一条 entry")
	assert.Equal(t, "bash", snap[0].Name)
	assert.Equal(t, `{"cmd":"ls"}`, snap[0].Args)
	assert.True(t, snap[0].Flushed,
		"看到 index=1 时, index=0 必须立刻被 flush (实时聚合特性)")
	assert.Equal(t, "webfetch", snap[1].Name)
	assert.Equal(t, `{"url":"https://x"}`, snap[1].Args)
	assert.False(t, snap[1].Flushed, "index=1 还未 Flush() 调用时不应被 flush")

	a.Flush()
	snap2 := a.Snapshot()
	assert.True(t, snap2[1].Flushed, "Flush() 必须把剩余 index=1 也 flush")
}

func TestToolCallAgg_Flush_Idempotent(t *testing.T) {
	t.Setenv("AIBALANCE_TOOL_CALL_AGG", "on")
	a := NewToolCallAggregator("idem")
	require.NotNil(t, a)

	a.Observe([]*aispec.ToolCall{{
		Index:    0,
		ID:       "x",
		Type:     "function",
		Function: aispec.FuncReturn{Name: "foo", Arguments: `{}`},
	}})

	a.Flush()
	first := a.Snapshot()
	a.Flush() // 第二次 Flush 不能 panic 不能改变状态
	second := a.Snapshot()
	assert.Equal(t, first, second, "Flush 必须幂等, snapshot 一致")
}

func TestToolCallAgg_NameMissing_StillSnapshotted(t *testing.T) {
	t.Setenv("AIBALANCE_TOOL_CALL_AGG", "on")
	a := NewToolCallAggregator("noname")
	require.NotNil(t, a)

	// 只有 arguments, 没 name (异常上游, 模拟 aispec name=="" && args=="" 跳过逻辑
	// 之后那种边缘 case)
	a.Observe([]*aispec.ToolCall{{
		Index:    0,
		Function: aispec.FuncReturn{Arguments: `{"x":1}`},
	}})
	a.Flush()
	snap := a.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "", snap[0].Name, "name 缺失时聚合器不强行填值, 仅记录原状")
	assert.Equal(t, `{"x":1}`, snap[0].Args)
	assert.True(t, snap[0].Flushed)
}

func TestToolCallAgg_NilSafe(t *testing.T) {
	t.Setenv("AIBALANCE_TOOL_CALL_AGG", "off")
	var a *ToolCallAggregator
	assert.NotPanics(t, func() {
		a.Observe([]*aispec.ToolCall{{Index: 0}})
		a.Flush()
		assert.Nil(t, a.Snapshot())
	})
}
