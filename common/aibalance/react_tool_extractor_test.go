package aibalance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// react_tool_extractor_test.go 覆盖 ReactToolExtractor 的所有边界 case.
//
// 关键词: aibalance react tool extractor 单测, 跨 chunk / 并行 tool / 坏 JSON / partial prefix

// extractorRecorder 是一个 helper, 用来在测试里收集 extractor emit 的 text 与 tool_call.
type extractorRecorder struct {
	textSB    strings.Builder
	toolCalls []*aispec.ToolCall
}

func (r *extractorRecorder) onContent(p []byte) error {
	r.textSB.Write(p)
	return nil
}
func (r *extractorRecorder) onToolCall(tc *aispec.ToolCall) error {
	r.toolCalls = append(r.toolCalls, tc)
	return nil
}

func newRec() *extractorRecorder { return &extractorRecorder{} }

func newExtractorWithRec(r *extractorRecorder) *ReactToolExtractor {
	return NewReactToolExtractor(r.onContent, r.onToolCall)
}

// 关键词: pure text passthrough, 无 tool_call 标签
func TestReactExtractor_PureText(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	require.NoError(t, e.Write([]byte("Hello world, no tool call here.")))
	require.NoError(t, e.Flush())
	assert.Equal(t, "Hello world, no tool call here.", r.textSB.String())
	assert.Empty(t, r.toolCalls)
}

// 关键词: single tool_call 一次写入完整
func TestReactExtractor_SingleToolCallOneShot(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name=get_weather]{"city":"BJ"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Empty(t, r.textSB.String(), "no extra text expected")
	require.Equal(t, 1, len(r.toolCalls))
	tc := r.toolCalls[0]
	assert.Equal(t, 0, tc.Index)
	assert.Equal(t, "function", tc.Type)
	assert.Equal(t, "get_weather", tc.Function.Name)
	assert.Equal(t, `{"city":"BJ"}`, tc.Function.Arguments)
	assert.True(t, strings.HasPrefix(tc.ID, "call_react_"))
}

// 关键词: tool_call 跨 chunk 切分, 每 chunk 仅 1-3 字节
func TestReactExtractor_SingleToolCallTinyChunks(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name=get_weather]{"city":"BJ"}[/tool_call]`
	for i := 0; i < len(body); i += 3 {
		j := i + 3
		if j > len(body) {
			j = len(body)
		}
		require.NoError(t, e.Write([]byte(body[i:j])))
	}
	require.NoError(t, e.Flush())
	assert.Empty(t, r.textSB.String())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "get_weather", r.toolCalls[0].Function.Name)
	assert.Equal(t, `{"city":"BJ"}`, r.toolCalls[0].Function.Arguments)
}

// 关键词: parallel multi tool_call 并行
func TestReactExtractor_ParallelToolCalls(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name=t1]{"a":1}[/tool_call][tool_call name=t2]{"b":2}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Empty(t, r.textSB.String())
	require.Equal(t, 2, len(r.toolCalls))
	assert.Equal(t, 0, r.toolCalls[0].Index)
	assert.Equal(t, "t1", r.toolCalls[0].Function.Name)
	assert.Equal(t, `{"a":1}`, r.toolCalls[0].Function.Arguments)
	assert.Equal(t, 1, r.toolCalls[1].Index)
	assert.Equal(t, "t2", r.toolCalls[1].Function.Name)
	assert.Equal(t, `{"b":2}`, r.toolCalls[1].Function.Arguments)
}

// 关键词: leading thinking text + tool_call, 前导文本要透传给 client
func TestReactExtractor_LeadingTextThenToolCall(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `Let me think. I will call get_weather.
[tool_call name=get_weather]{"city":"BJ"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Contains(t, r.textSB.String(), "Let me think. I will call get_weather.")
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "get_weather", r.toolCalls[0].Function.Name)
}

// 关键词: bad JSON tool_call, 坏数据兜底为 text emit
func TestReactExtractor_BadJsonFallbackToText(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name=bad]{unclosed[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Contains(t, r.textSB.String(), `[tool_call name=bad]`, "fallback raw text")
	assert.Empty(t, r.toolCalls)
}

// 关键词: missing close tag, 流末 Flush 把残段当 text emit
func TestReactExtractor_UnclosedAtFlush(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `prefix text [tool_call name=oops]{"x":1}` // 缺 [/tool_call]
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Contains(t, r.textSB.String(), "prefix text ")
	assert.Contains(t, r.textSB.String(), `[tool_call name=oops]`)
	assert.Empty(t, r.toolCalls)
}

// 关键词: stray '[' 字符, 不应触发误匹配
func TestReactExtractor_StrayBracketInText(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `Here are some random brackets [1, 2, 3] in normal text.`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Equal(t, body, r.textSB.String())
	assert.Empty(t, r.toolCalls)
}

// 关键词: tool_call 后接 trailing text, 一并透传给 client
func TestReactExtractor_TrailingTextAfterToolCall(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name=t1]{}[/tool_call]Some trailing reasoning text.`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Equal(t, "Some trailing reasoning text.", r.textSB.String())
	require.Equal(t, 1, len(r.toolCalls))
}

// 关键词: name 带双引号
func TestReactExtractor_QuotedName(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name="weather_v2"]{"city":"BJ"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "weather_v2", r.toolCalls[0].Function.Name)
}

// 关键词: empty arguments 视为 {}
func TestReactExtractor_EmptyArgsTreatedAsObject(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name=ping][/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "{}", r.toolCalls[0].Function.Arguments)
}

// 关键词: partial prefix 切分保护, 末尾 "[tool_" 字节不立即 emit
func TestReactExtractor_PartialPrefixSafety(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	require.NoError(t, e.Write([]byte("hello [tool_"))) // partial 前缀
	// 此时 OnContent 应该只看到 "hello "
	assert.Equal(t, "hello ", r.textSB.String())
	require.NoError(t, e.Write([]byte("call name=t1]{}[/tool_call]"))) // 补全
	require.NoError(t, e.Flush())
	assert.Equal(t, "hello ", r.textSB.String(), "no extra text after completion")
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "t1", r.toolCalls[0].Function.Name)
}

// 关键词: HasEmittedToolCall stats
func TestReactExtractor_HasEmittedToolCall(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	assert.False(t, e.HasEmittedToolCall())
	require.NoError(t, e.Write([]byte("plain text")))
	assert.False(t, e.HasEmittedToolCall())
	require.NoError(t, e.Write([]byte(`[tool_call name=p]{}[/tool_call]`)))
	assert.True(t, e.HasEmittedToolCall())
}

// 关键词: id 属性透传, round2 flatten 模型 mimic 格式
// 形态: [tool_call id="call_xyz" name="bash"]args[/tool_call]
// 解析后 ToolCall.ID 必须保留模型给出的 call_xyz, 不能被 "call_react_N" 覆盖
func TestReactExtractor_HeaderWithIdAttribute_PreservesModelId(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call id="call_xyz_123" name="bash"]{"command":"ls"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	tc := r.toolCalls[0]
	assert.Equal(t, "call_xyz_123", tc.ID,
		"模型主动给出 id=\"...\" 时应优先保留, 而不是覆盖成 call_react_N")
	assert.Equal(t, "bash", tc.Function.Name)
	assert.Equal(t, `{"command":"ls"}`, tc.Function.Arguments)
}

// 关键词: 模型 mimic 多种 id/name 顺序
// 形态: [tool_call name="bash" id="call_xyz"]args[/tool_call] (顺序倒置)
func TestReactExtractor_HeaderWithIdAttribute_OrderReversed(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name="bash" id="call_abc"]{"command":"pwd"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "call_abc", r.toolCalls[0].ID)
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
}

// 关键词: 模型未给 id, 回落到 call_react_N
func TestReactExtractor_HeaderWithoutId_FallbackToReactN(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name="bash"]{"command":"pwd"}[/tool_call][tool_call name="bash"]{"command":"id"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 2, len(r.toolCalls))
	assert.Equal(t, "call_react_0", r.toolCalls[0].ID)
	assert.Equal(t, "call_react_1", r.toolCalls[1].ID)
}

// 关键词: 多 tool_call 并行, 每个 id 独立, 不互相串号
func TestReactExtractor_ParallelToolCallsWithIds(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `Let me probe.` +
		`[tool_call id="call_01" name="bash"]{"command":"curl /a"}[/tool_call]` +
		`[tool_call id="call_02" name="bash"]{"command":"curl /b"}[/tool_call]` +
		`[tool_call id="call_03" name="bash"]{"command":"curl /c"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 3, len(r.toolCalls))
	assert.Equal(t, "Let me probe.", r.textSB.String(), "leading text 必须透传")
	assert.Equal(t, "call_01", r.toolCalls[0].ID)
	assert.Equal(t, "call_02", r.toolCalls[1].ID)
	assert.Equal(t, "call_03", r.toolCalls[2].ID)
	assert.Equal(t, 0, r.toolCalls[0].Index, "并行 index 0 隔离")
	assert.Equal(t, 1, r.toolCalls[1].Index, "并行 index 1 隔离")
	assert.Equal(t, 2, r.toolCalls[2].Index, "并行 index 2 隔离")
	assert.Equal(t, `{"command":"curl /a"}`, r.toolCalls[0].Function.Arguments)
	assert.Equal(t, `{"command":"curl /b"}`, r.toolCalls[1].Function.Arguments)
	assert.Equal(t, `{"command":"curl /c"}`, r.toolCalls[2].Function.Arguments)
}

// 关键词: 多 tool_call 跨 chunk 串行到达
func TestReactExtractor_ParallelToolCallsTinyChunks(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call id="c1" name="bash"]{"x":1}[/tool_call][tool_call id="c2" name="bash"]{"x":2}[/tool_call]`
	for i := 0; i < len(body); i += 5 {
		j := i + 5
		if j > len(body) {
			j = len(body)
		}
		require.NoError(t, e.Write([]byte(body[i:j])))
	}
	require.NoError(t, e.Flush())
	require.Equal(t, 2, len(r.toolCalls))
	assert.Equal(t, "c1", r.toolCalls[0].ID)
	assert.Equal(t, "c2", r.toolCalls[1].ID)
	assert.Equal(t, `{"x":1}`, r.toolCalls[0].Function.Arguments)
	assert.Equal(t, `{"x":2}`, r.toolCalls[1].Function.Arguments)
}

// 关键词: header 属性子串误中防护 (id="...name..." 不应触发 name 命中)
func TestReactExtractor_HeaderAttrSubstringSafety(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	// id 值里恰好包含 "name=" 子串, 不应被误当成 name 属性
	body := `[tool_call id="abc-name=user-1" name="bash"]{"x":1}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name,
		"name 解析必须从 header 的真实 name= 属性抠出, 而不是 id 值内的子串")
	assert.Equal(t, "abc-name=user-1", r.toolCalls[0].ID,
		"id 值原样保留, 包括内嵌的 = 字符")
}

// 关键词: round2 flatten 输出格式严格往返
// 校验 react extractor 能精确解析 flattenAssistantWithToolCalls 输出
// (id="..." + name="..." + args 多行) 的格式, 保证 round-trip 自洽.
func TestReactExtractor_ParsesFlattenAssistantOutput(t *testing.T) {
	// 与 round2_flatten.go 中 flattenAssistantWithToolCalls 写出的字符串完全一致
	body := "[tool_call id=\"call_xyz\" name=\"get_weather\"]\n" +
		"{\"city\":\"Beijing\"}\n" +
		"[/tool_call]\n"

	r := newRec()
	e := newExtractorWithRec(r)
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	tc := r.toolCalls[0]
	assert.Equal(t, "call_xyz", tc.ID)
	assert.Equal(t, "get_weather", tc.Function.Name)
	assert.Equal(t, `{"city":"Beijing"}`, tc.Function.Arguments,
		"args 前后换行必须被 TrimSpace 掉, 与 JSON 客户端期望对齐")
}

// 关键词: overflow protection, 没闭合且超长 -> fallback text
func TestReactExtractor_OverflowFallback(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	bigJunk := strings.Repeat("x", extractorBufferLimit+1024)
	body := `[tool_call name=bad]` + bigJunk // 永远不闭合
	require.NoError(t, e.Write([]byte(body)))
	// 触发 overflow fallback (drainLocked 内部判定)
	require.NoError(t, e.Flush())
	assert.Contains(t, r.textSB.String(), `[tool_call name=bad]`, "overflow content fall back to text")
	assert.Empty(t, r.toolCalls)
}
