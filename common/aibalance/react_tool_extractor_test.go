package aibalance

import (
	"encoding/json"
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

// 关键词: 尖括号 XML 风格 tool_call, deepseek 在 react 模式下漂移成 <tool_call ...> 形态
// 复现用户在 opencode TUI 里看到的真实失败现象 (截图证据):
//
//   <tool_call id="call_kxwszjdch9e2h7cbfoj8q43y" name="bash">
//   {"command":"curl -sk --max-time 10 -X GET https://id.redhaze.top 2>&1 | head -80","description":"GET request to see page content"}</tool_call>
//
// 模型不严格遵守 system prompt 要求的方括号 [tool_call ...]...[/tool_call],
// 自由发挥成 XML 尖括号. extractor 必须兼容这种漂移, 否则原样作为 content 透传给客户端,
// opencode 渲染成普通文字、不调用 bash 工具.
//
// 关键词: 尖括号 tool_call, XML 风格漂移, deepseek hallucinate, opencode TUI 失败修复
func TestReactExtractor_AngleBracketSingleToolCall(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call id="call_kxwszjdch9e2h7cbfoj8q43y" name="bash">
{"command":"curl -sk --max-time 10 -X GET https://id.redhaze.top 2>&1 | head -80","description":"GET request to see page content"}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Empty(t, r.textSB.String(), "尖括号 tool_call 应被识别, 不残留为 content text")
	require.Equal(t, 1, len(r.toolCalls))
	tc := r.toolCalls[0]
	assert.Equal(t, "call_kxwszjdch9e2h7cbfoj8q43y", tc.ID)
	assert.Equal(t, "bash", tc.Function.Name)
	assert.Equal(t, `{"command":"curl -sk --max-time 10 -X GET https://id.redhaze.top 2>&1 | head -80","description":"GET request to see page content"}`,
		tc.Function.Arguments)
}

// 关键词: 尖括号格式无 id 属性回落 call_react_N
func TestReactExtractor_AngleBracketWithoutIdFallback(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call name="get_weather">{"city":"BJ"}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "call_react_0", r.toolCalls[0].ID)
	assert.Equal(t, "get_weather", r.toolCalls[0].Function.Name)
}

// 关键词: 尖括号并行多 tool_call
func TestReactExtractor_AngleBracketParallel(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `Let me probe.` +
		`<tool_call id="call_a" name="bash">{"command":"curl /a"}</tool_call>` +
		`<tool_call id="call_b" name="bash">{"command":"curl /b"}</tool_call>` +
		`<tool_call id="call_c" name="bash">{"command":"curl /c"}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 3, len(r.toolCalls))
	assert.Equal(t, "Let me probe.", r.textSB.String(), "leading text 必须透传")
	assert.Equal(t, "call_a", r.toolCalls[0].ID)
	assert.Equal(t, "call_b", r.toolCalls[1].ID)
	assert.Equal(t, "call_c", r.toolCalls[2].ID)
	assert.Equal(t, 0, r.toolCalls[0].Index)
	assert.Equal(t, 1, r.toolCalls[1].Index)
	assert.Equal(t, 2, r.toolCalls[2].Index)
}

// 关键词: 尖括号格式跨 chunk 切分
func TestReactExtractor_AngleBracketTinyChunks(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call id="c1" name="bash">{"x":1}</tool_call>`
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
	assert.Equal(t, "c1", r.toolCalls[0].ID)
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
	assert.Equal(t, `{"x":1}`, r.toolCalls[0].Function.Arguments)
}

// 关键词: 方括号与尖括号混合 (一份响应里两种格式都出现)
func TestReactExtractor_MixedAngleAndBracket(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call id="b1" name="bash"]{"x":1}[/tool_call]` +
		`<tool_call id="a1" name="bash">{"x":2}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 2, len(r.toolCalls))
	assert.Equal(t, "b1", r.toolCalls[0].ID)
	assert.Equal(t, "a1", r.toolCalls[1].ID)
}

// 关键词: 尖括号 partial prefix 防护. "<tool_" 尾部不能立刻 emit
func TestReactExtractor_AngleBracketPartialPrefixSafety(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	require.NoError(t, e.Write([]byte("hello <tool_")))
	assert.Equal(t, "hello ", r.textSB.String())
	require.NoError(t, e.Write([]byte(`call name="t1">{}</tool_call>`)))
	require.NoError(t, e.Flush())
	assert.Equal(t, "hello ", r.textSB.String())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "t1", r.toolCalls[0].Function.Name)
}

// 关键词: 普通 HTML/XML 文本里出现的 < 字符不应触发 tool_call 误判.
// 例: "use < instead of >" 这种纯文本必须原样透传.
func TestReactExtractor_StrayAngleBracketInText(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `Compare a < b and c > d. <html> tags should stay intact.`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Equal(t, body, r.textSB.String())
	assert.Empty(t, r.toolCalls)
}

// 关键词: 尖括号坏 JSON, 兜底 text
func TestReactExtractor_AngleBracketBadJsonFallback(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call name="bad">{unclosed</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Contains(t, r.textSB.String(), `<tool_call name="bad">`)
	assert.Empty(t, r.toolCalls)
}

// ----- 混合格式 (deepseek hallucinate 变体) 边界覆盖 -----
//
// 用户 opencode TUI 第二次复现, 模型实际输出:
//
//   [tool_call id="call_00_C3Fw8vLYnauB9r4kJrfCpK3Q" name="bash">
//   {...JSON 含 < > ] 字符...}
//   [/tool_call]
//
// 即 open=`[tool_call`, header_end=`>`, close=`[/tool_call]` 三段错位混合.
// 任何穷举式 variant 都不能预先知道全部组合 (open 2 种 x header_end 2 种 x
// close 4 种 = 16 种), 必须三段独立解析 + quote-aware header end + args 字符
// 透明处理.
//
// 关键词: hallucinate 混合格式, 三段独立解析, quote-aware header end,
//        args 含 < > ] 字符防误切

// 关键词: 截图2 复现, [open + > header end + [/close 混合
func TestReactExtractor_MixedBracketOpenAngleHeaderEnd(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call id="call_00_C3Fw8vLYnauB9r4kJrfCpK3Q" name="bash">
{"command":"bash -c '\n# SSL cert info\nopenssl s_client -connect redhaze.top:443 -servername redhaze.top </dev/null 2>/dev/null | openssl x509 -noout -text 2>/dev/null | grep -E \"Subject:|DNS:|Issuer:|Not After\"\n'", "description":"SSL certificate details for redhaze.top"}
[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls), "混合格式必须被识别")
	tc := r.toolCalls[0]
	assert.Equal(t, "call_00_C3Fw8vLYnauB9r4kJrfCpK3Q", tc.ID)
	assert.Equal(t, "bash", tc.Function.Name)
	// args 体里含的 < > 字符必须原样保留, 因为它们是 JSON value 的一部分
	assert.Contains(t, tc.Function.Arguments, `</dev/null`,
		"args body 里的 </dev/null 必须原样保留, 不能被当成 close 标签误切")
	assert.Contains(t, tc.Function.Arguments, `2>/dev/null`,
		"args body 里的 2>/dev/null 必须原样保留")
	assert.Contains(t, tc.Function.Arguments, `Not After`,
		"args body 完整保留")
	// args 必须是合法 JSON
	var probe map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Function.Arguments), &probe))
	assert.Equal(t, "SSL certificate details for redhaze.top", probe["description"])
}

// 关键词: <open + ] header end + [/close 混合
func TestReactExtractor_MixedAngleOpenBracketHeaderEnd(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call id="c1" name="bash"]{"command":"ls"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	tc := r.toolCalls[0]
	assert.Equal(t, "c1", tc.ID)
	assert.Equal(t, "bash", tc.Function.Name)
	assert.Equal(t, `{"command":"ls"}`, tc.Function.Arguments)
}

// 关键词: [open + ] header end + </close 混合
func TestReactExtractor_MixedBracketOpenAngleClose(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call id="c2" name="bash"]{"command":"pwd"}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	tc := r.toolCalls[0]
	assert.Equal(t, "c2", tc.ID)
	assert.Equal(t, "bash", tc.Function.Name)
	assert.Equal(t, `{"command":"pwd"}`, tc.Function.Arguments)
}

// 关键词: <open + > header end + [/close 混合
func TestReactExtractor_MixedAngleOpenBracketClose(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call id="c3" name="bash">{"command":"id"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	tc := r.toolCalls[0]
	assert.Equal(t, "c3", tc.ID)
	assert.Equal(t, "id", tc.Function.Arguments[len(tc.Function.Arguments)-4:len(tc.Function.Arguments)-2])
	assert.Equal(t, "bash", tc.Function.Name)
}

// 关键词: args 字符串内含 [/tool_call] 子串 (用户写脚本时引用了关键字),
// 不能被误识别为 close 标签提前切割.
//
// 注意: 这是已知边界 case. 当前实现按"最早 close"切割, 如果 args body 里
// 出现完整的 [/tool_call] 子串, 会被提前切. JSON 校验会失败 (因为 args 切残),
// 然后整段 fall back 成 text. 测试只断言 "不会让客户端解析出残缺 args 调用",
// 不强求识别成功 -- 这是上游模型自己生成 args 时该回避的字符.
func TestReactExtractor_ArgsContainingCloseTokenSubstringDoesNotProduceCorruptCall(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	// args 描述里故意包含 "[/tool_call]" 字面量, 模拟模型在 description 里抄袭 prompt 的极端情况
	body := `[tool_call id="cx" name="bash"]{"command":"echo done","description":"prints [/tool_call] in log"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	for _, tc := range r.toolCalls {
		// 任何识别出的 tool_call 都必须有合法 JSON args, 不能给客户端发残缺 JSON
		var probe map[string]any
		assert.NoErrorf(t, json.Unmarshal([]byte(tc.Function.Arguments), &probe),
			"identified tool_call must carry valid JSON args, got: %s", tc.Function.Arguments)
	}
}

// 关键词: header attr value 内含 ] 或 > 字符, quote-aware 必须跳过引号内的特殊字符
func TestReactExtractor_HeaderAttrValueContainsHeaderEndChar(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	// id 值里故意藏 `>` 与 `]`, 但因为是 "..." 引号包裹, 不应该当 header end
	body := `<tool_call id="weird>id]value" name="bash">{"command":"ls"}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "weird>id]value", r.toolCalls[0].ID,
		"引号内的 > ] 必须保留进 id, 不能被当成 header end 切断")
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
	assert.Equal(t, `{"command":"ls"}`, r.toolCalls[0].Function.Arguments)
}

// 关键词: 多条混合格式并行, 各 variant 共存于一次响应
func TestReactExtractor_MultipleMixedVariantsParallel(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `lead.` +
		// variant 1: 规范方括号
		`[tool_call id="b1" name="bash"]{"x":1}[/tool_call]` +
		// variant 2: 截图1 纯尖括号
		`<tool_call id="a1" name="bash">{"x":2}</tool_call>` +
		// variant 3: 截图2 [open + > headerEnd + [/close
		`[tool_call id="m1" name="bash">{"x":3}[/tool_call]` +
		// variant 4: <open + ] headerEnd + </close
		`<tool_call id="m2" name="bash"]{"x":4}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 4, len(r.toolCalls), "4 种混合格式都要识别")
	assert.Equal(t, "lead.", r.textSB.String())
	assert.Equal(t, []string{"b1", "a1", "m1", "m2"}, []string{
		r.toolCalls[0].ID, r.toolCalls[1].ID, r.toolCalls[2].ID, r.toolCalls[3].ID,
	})
	assert.Equal(t, []int{0, 1, 2, 3}, []int{
		r.toolCalls[0].Index, r.toolCalls[1].Index, r.toolCalls[2].Index, r.toolCalls[3].Index,
	})
	for i, tc := range r.toolCalls {
		var probe map[string]any
		require.NoErrorf(t, json.Unmarshal([]byte(tc.Function.Arguments), &probe),
			"call #%d args must be valid JSON: %s", i, tc.Function.Arguments)
		assert.Equal(t, float64(i+1), probe["x"])
	}
}

// 关键词: 截图2 完整 multi-line + reasoning + tool_call 跨 chunk
// 模拟真实 SSE: 模型先输出推理文本, 然后 tool_call header 与 args 跨多个 chunk 到达
func TestReactExtractor_MixedFormatTinyChunks(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `Interesting. Let me dig.
[tool_call id="call_C3Fw8vL" name="bash">
{"command":"openssl s_client -connect a.com:443 </dev/null 2>/dev/null","description":"probe TLS"}
[/tool_call]`
	for i := 0; i < len(body); i += 4 {
		j := i + 4
		if j > len(body) {
			j = len(body)
		}
		require.NoError(t, e.Write([]byte(body[i:j])))
	}
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	tc := r.toolCalls[0]
	assert.Equal(t, "call_C3Fw8vL", tc.ID)
	assert.Equal(t, "bash", tc.Function.Name)
	var probe map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Function.Arguments), &probe))
	assert.Contains(t, probe["command"], `</dev/null`)
	assert.Contains(t, probe["command"], `2>/dev/null`)
	assert.Contains(t, r.textSB.String(), "Interesting. Let me dig.")
}

// 关键词: 单引号 attr value 也要 quote-aware
func TestReactExtractor_HeaderSingleQuotedAttr(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call id='c-sq' name='bash'>{"x":1}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Equal(t, 1, len(r.toolCalls))
	assert.Equal(t, "c-sq", r.toolCalls[0].ID)
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
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
