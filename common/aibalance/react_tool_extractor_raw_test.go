package aibalance

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// react_tool_extractor_raw_test.go 覆盖 v2 raw passthrough 引入的 5 种新 variant:
//   1. anthropic-xml-parameter: <tool_call name="X"><parameter name="K">V</parameter>...</tool_call>
//   2. chinese-invoke:          [调用 NAME] {...} [/tool_call]
//   3. hermes-body-name:        <tool_call>{"name":"X","arguments":{...}}</tool_call>
//   4. deepseek-fullwidth:      <｜tool_calls_begin｜>...<｜tool_calls_end｜> 含多个子帧
//   5. mistral-toolcalls:       [TOOL_CALLS] [{"name":"X","arguments":{...}}, ...]
//
// 以及 v2 raw passthrough 哲学下的关键回归 case:
//   - 跨 chunk 边界 (中文 / 全角 / mistral token 的 partial prefix 保护)
//   - name 抠不出来时 fall back to plain text 的兜底行为
//   - 向后兼容: canonical bracket / angle 路径不变
//
// 关键词: v2 raw passthrough 单测, 5 variants, 跨 chunk, hallucinate fallback,
//   anthropic xml param / chinese invoke / hermes body / deepseek fullwidth / mistral

// ============================================================================
// 1. Anthropic XML parameter (用户截图1 复现)
// ============================================================================

// 用户截图1 的真实形态: model hallucinate 输出了 Anthropic Claude 风格的
// <parameter name="K">V</parameter> 嵌套 XML 作为 args body. v1 会 fall back
// to plain text, v2 raw passthrough 把整段 XML body 原文塞进 Arguments 透传.
//
// 关键词: 截图1 anthropic XML parameter 复现, args body 原文透传
func TestRawExtract_AnthropicXMLParameter_UserScreenshot1(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call name="bash">
<parameter name="command">curl -sI -o /dev/null -w "HTTP %{http_code} (%{size_download}B)  %{url_effective}\n" "http://192.168.3.24:18080/" "http://192.168.3.24:18080/portal" "http://192.168.3.24:18080/api"</parameter>
<parameter name="description">Probe multiple endpoints and ports for status</parameter>
<parameter name="timeout">30000</parameter>
</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 1, "v2: anthropic XML parameter 必须被识别为单个 ToolCall")
	tc := r.toolCalls[0]
	assert.Equal(t, "bash", tc.Function.Name, "name 从 header attr name=\"bash\" 抠出")
	assert.Contains(t, tc.Function.Arguments, `<parameter name="command">curl`,
		"args body 原文透传, 含 <parameter> XML 标签")
	assert.Contains(t, tc.Function.Arguments, `<parameter name="timeout">30000</parameter>`,
		"args body 完整保留所有 <parameter> 元素")
	assert.Empty(t, r.textSB.String(), "整段不应 fall back to plain text content")
}

// ============================================================================
// 2. Chinese invoke (用户截图2 复现)
// ============================================================================

// 用户截图2 的真实形态: model hallucinate 输出 [调用 NAME] open 标签 (中文动词)
// + 标准 JSON args + [/tool_call] close. v1 因为不识别中文 open token 完全没用,
// v2 把 [调用 加进 openTokens 候选 + 加特例 name 提取.
//
// 关键词: 截图2 chinese invoke 复现, 中文动词 open token
func TestRawExtract_ChineseInvoke_UserScreenshot2(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[调用 todowrite] {"todos":[{"content":"hello","status":"pending"},{"content":"world","status":"in_progress"}]} [/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 1, "v2: chinese invoke 必须被识别")
	tc := r.toolCalls[0]
	assert.Equal(t, "todowrite", tc.Function.Name, "name 从 [调用 之后取")
	// args body 原文 (含 leading / trailing 空白), TrimSpace 后比较关键内容
	args := strings.TrimSpace(tc.Function.Arguments)
	assert.True(t, strings.HasPrefix(args, `{"todos":`),
		"args body 原文透传, 含完整 JSON")
	assert.Contains(t, args, `"status":"in_progress"`)
}

// chinese invoke 同时跑通"无空白前缀"形态 [调用NAME], 但是这种因为 plan
// 设计的 token 是 "[调用"(无尾随空白), 仍然能识别 — name 取 inner 第一字段直到 ']'
//
// 关键词: chinese invoke 无空白前缀边界
func TestRawExtract_ChineseInvoke_NoLeadingSpace(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[调用web_search]{"q":"yaklang"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 1)
	assert.Equal(t, "web_search", r.toolCalls[0].Function.Name)
}

// ============================================================================
// 3. Hermes / Qwen2.5 / QwQ body-name
// ============================================================================

// Hermes / Qwen2.5 / QwQ-32B / Hermes-3 全家通用的 body-name 形态:
//   <tool_call>{"name":"X","arguments":{...}}</tool_call>
//
// header 没有 name= attr, name 在 JSON body 的 .name 字段里, args 在 .arguments 子段.
// extractor 探测 header 没 name= 时回退到 hermes-body-name 解析.
//
// 关键词: hermes body name, qwen2.5 兼容, header 无 name= 回退
func TestRawExtract_HermesBodyName_Basic(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call>{"name":"bash","arguments":{"command":"ls -la","timeout":15}}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 1, "hermes body-name 必须被识别")
	tc := r.toolCalls[0]
	assert.Equal(t, "bash", tc.Function.Name, "name 从 .name 字段抠出")
	// .arguments 子段被剥出来当 args (恢复 native 协议语义)
	var probe map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Function.Arguments), &probe),
		"hermes body-name 模式下 args 应该是 .arguments 子段 (JSON object)")
	assert.Equal(t, "ls -la", probe["command"])
	assert.Equal(t, float64(15), probe["timeout"])
}

// hermes body-name 兼容 .arguments 子段是 string 形态 (双重 escape JSON).
// 某些 wrapper 把 arguments 序列化成 JSON string 而不是 nested object.
//
// 关键词: hermes body name args string 形态, 双重 escape JSON
func TestRawExtract_HermesBodyName_ArgumentsAsString(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call>{"name":"bash","arguments":"{\"command\":\"ls\"}"}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 1)
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
	// args 是 unescape 后的 JSON object 字符串
	assert.Equal(t, `{"command":"ls"}`, r.toolCalls[0].Function.Arguments)
}

// hermes body-name 兼容 header 同时带 id= 但没 name= 的形态.
//
// 关键词: hermes body name with id attribute
func TestRawExtract_HermesBodyName_WithIdAttribute(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call id="call_abc">{"name":"bash","arguments":{"command":"pwd"}}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 1)
	tc := r.toolCalls[0]
	assert.Equal(t, "call_abc", tc.ID, "id 从 header attr 保留")
	assert.Equal(t, "bash", tc.Function.Name)
	assert.Equal(t, `{"command":"pwd"}`, tc.Function.Arguments)
}

// ============================================================================
// 4. DeepSeek V3.1 全角分隔符
// ============================================================================

// DeepSeek V3.1 官方 tool calling 格式:
//   <｜tool_calls_begin｜>
//     <｜tool_call_begin｜>NAME<｜tool_sep｜>ARGS_JSON<｜tool_call_end｜>
//     ...(更多并行子帧)...
//   <｜tool_calls_end｜>
//
// 注意: ｜ 是全角 U+FF5C (UTF-8 3 bytes), 不是 ASCII '|'.
//
// 关键词: deepseek v31 fullwidth, 全角分隔符, single tool call
func TestRawExtract_DeepseekFullwidth_SingleCall(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<｜tool_calls_begin｜><｜tool_call_begin｜>bash<｜tool_sep｜>{"command":"echo hello"}<｜tool_call_end｜><｜tool_calls_end｜>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 1, "deepseek-fullwidth single call 必须被识别")
	tc := r.toolCalls[0]
	assert.Equal(t, "bash", tc.Function.Name)
	assert.Equal(t, `{"command":"echo hello"}`, tc.Function.Arguments)
}

// 并行多 tool_call: 一个外层 calls_begin / calls_end 含多个 sub-frame.
//
// 关键词: deepseek v31 fullwidth, 并行多 tool_call
func TestRawExtract_DeepseekFullwidth_Parallel(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<｜tool_calls_begin｜>` +
		`<｜tool_call_begin｜>bash<｜tool_sep｜>{"command":"echo step-1"}<｜tool_call_end｜>` +
		`<｜tool_call_begin｜>bash<｜tool_sep｜>{"command":"echo step-2"}<｜tool_call_end｜>` +
		`<｜tool_call_begin｜>todowrite<｜tool_sep｜>{"todos":[]}<｜tool_call_end｜>` +
		`<｜tool_calls_end｜>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 3, "deepseek-fullwidth 并行 3 个 sub-frame 必须各自 emit")
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
	assert.Equal(t, `{"command":"echo step-1"}`, r.toolCalls[0].Function.Arguments)
	assert.Equal(t, "bash", r.toolCalls[1].Function.Name)
	assert.Equal(t, `{"command":"echo step-2"}`, r.toolCalls[1].Function.Arguments)
	assert.Equal(t, "todowrite", r.toolCalls[2].Function.Name)
	assert.Equal(t, `{"todos":[]}`, r.toolCalls[2].Function.Arguments)
	// index 应该按顺序 0/1/2
	assert.Equal(t, 0, r.toolCalls[0].Index)
	assert.Equal(t, 1, r.toolCalls[1].Index)
	assert.Equal(t, 2, r.toolCalls[2].Index)
}

// ============================================================================
// 5. Mistral [TOOL_CALLS] 数组形态
// ============================================================================

// Mistral 官方格式: [TOOL_CALLS] 后跟 JSON 数组, 每个 element 是
// {"name":"X","arguments":{...}}. 数组本身是 JSON 数组括号自闭合, extractor
// 用 brace-balanced + quote-aware 扫描定位结尾 ']'.
//
// 关键词: mistral toolcalls array, brace-balanced JSON 扫描
func TestRawExtract_MistralArray_TwoCalls(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[TOOL_CALLS] [{"name":"bash","arguments":{"command":"ls"}},{"name":"todowrite","arguments":{"x":1}}]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 2, "mistral array 必须 emit N 个 ToolCall (N=element 数)")
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
	assert.Equal(t, `{"command":"ls"}`, r.toolCalls[0].Function.Arguments)
	assert.Equal(t, "todowrite", r.toolCalls[1].Function.Name)
	assert.Equal(t, `{"x":1}`, r.toolCalls[1].Function.Arguments)
}

// mistral 跨 chunk: 数组未完整接收时 extractor 应当 hold 不消费, 等下一次 Write.
//
// 关键词: mistral cross chunk, JSON array 跨 chunk 边界
func TestRawExtract_MistralArray_CrossChunk(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	chunk1 := `[TOOL_CALLS] [{"name":"bash","arguments":{"command":"l`
	chunk2 := `s -la"}}]`
	require.NoError(t, e.Write([]byte(chunk1)))
	// chunk1 时数组没收完, extractor 应当 hold, 不 emit 任何 ToolCall, 也不 emit text
	assert.Empty(t, r.toolCalls, "chunk1 时数组未完整, 不能提前 emit")
	assert.Empty(t, r.textSB.String(), "chunk1 时也不能把 partial 当 text emit")
	require.NoError(t, e.Write([]byte(chunk2)))
	require.NoError(t, e.Flush())
	require.Len(t, r.toolCalls, 1, "chunk2 来齐后必须 emit 1 个 ToolCall")
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
	assert.Equal(t, `{"command":"ls -la"}`, r.toolCalls[0].Function.Arguments)
}

// mistral 数组里含字符串带 ']' 字符, 需要 quote-aware 扫描不被误切.
//
// 关键词: mistral array quote-aware, args 含 close bracket 子串
func TestRawExtract_MistralArray_QuoteAwareCloseBracket(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[TOOL_CALLS] [{"name":"bash","arguments":{"command":"grep ']' file.txt"}}]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())

	require.Len(t, r.toolCalls, 1)
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
	assert.Contains(t, r.toolCalls[0].Function.Arguments, `grep ']' file.txt`,
		"args 里的 ']' 字符不能被误识别为数组 close")
}

// ============================================================================
// 跨 chunk 边界保护: chinese / 全角 / mistral 三类新 open token
// ============================================================================

// 中文 open token `[调用` 占 7 bytes (1 ASCII + 2 个 3-byte UTF-8 字符),
// 跨 chunk 拆开后 safeEmitTextLen 必须保留尾部 partial prefix.
//
// 关键词: chinese cross chunk partial prefix, UTF-8 多字节 token 保护
func TestRawExtract_ChineseInvoke_CrossChunk(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	// 整段: "prefix text [调用 web_search]{"q":"yaklang"}[/tool_call]"
	// 在 `[` 之后立即切分, 把 `调用` 拆到下一个 chunk
	chunk1 := `prefix text [`
	chunk2 := `调用 web_search]{"q":"yaklang"}[/tool_call]`
	require.NoError(t, e.Write([]byte(chunk1)))
	// chunk1 时 `[` 是 partial prefix 候选, 应保留, 不能误 emit
	assert.Equal(t, "prefix text ", r.textSB.String(),
		"chunk1 时 `[` 是 open token 候选 partial, 保留不 emit")
	require.NoError(t, e.Write([]byte(chunk2)))
	require.NoError(t, e.Flush())
	require.Len(t, r.toolCalls, 1)
	assert.Equal(t, "web_search", r.toolCalls[0].Function.Name)
}

// DeepSeek 全角分隔符 `<｜` 占 4 bytes, 跨 chunk 拆开同样要保护.
//
// 关键词: deepseek fullwidth cross chunk, 全角分隔符 partial prefix
func TestRawExtract_DeepseekFullwidth_CrossChunkAtAngle(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	full := `<｜tool_calls_begin｜><｜tool_call_begin｜>bash<｜tool_sep｜>{"command":"ls"}<｜tool_call_end｜><｜tool_calls_end｜>`
	// 在第一个 `<` 之后立刻切, partial prefix 是 `<`
	chunk1 := full[:1] // "<"
	chunk2 := full[1:]
	require.NoError(t, e.Write([]byte(chunk1)))
	require.NoError(t, e.Write([]byte(chunk2)))
	require.NoError(t, e.Flush())
	require.Len(t, r.toolCalls, 1)
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
}

// Mistral `[TOOL_CALLS]` open token 跨 chunk 边界保护.
//
// 关键词: mistral open token cross chunk
func TestRawExtract_MistralArray_CrossChunkAtOpenToken(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	chunk1 := `[TOOL_`
	chunk2 := `CALLS] [{"name":"bash","arguments":{"x":1}}]`
	require.NoError(t, e.Write([]byte(chunk1)))
	require.NoError(t, e.Write([]byte(chunk2)))
	require.NoError(t, e.Flush())
	require.Len(t, r.toolCalls, 1)
	assert.Equal(t, "bash", r.toolCalls[0].Function.Name)
}

// ============================================================================
// 兜底: name 抠不出来时 fall back to plain text
// ============================================================================

// 故意构造一个 [tool_call] 段没有 name= 也不是 hermes-body 形态:
// args body 不是 JSON object 含 .name, header 也没 name=. 应当整段当 text emit.
//
// 关键词: name 抠不出, fall back to plain text, v2 raw passthrough 兜底
func TestRawExtract_NameUnextractable_FallsBackToText(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	// header 只有 id, args body 是裸 string (非 JSON object 含 name)
	body := `[tool_call id="x"]"plain string body"[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	// hermes body 也解不出 (不是 object 含 .name), 整段 fall back to text
	assert.Empty(t, r.toolCalls, "name 抠不出来时不应 emit ToolCall")
	assert.Contains(t, r.textSB.String(), `[tool_call id="x"]`,
		"name 抠不出来时整段 fall back to plain text content")
}

// chinese invoke 形态但 name 是空 (`[调用 ]`), 也应当 fall back to text.
//
// 关键词: chinese invoke empty name fall back
func TestRawExtract_ChineseInvoke_EmptyName_FallsBackToText(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[调用 ] {"x":1} [/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	assert.Empty(t, r.toolCalls, "空 name 不应 emit ToolCall")
	assert.Contains(t, r.textSB.String(), `[调用 ]`)
}

// ============================================================================
// 向后兼容: canonical bracket / angle 形态 args 仍是 JSON 字符串
// ============================================================================

// canonical bracket: 老 prompt 规定的格式仍然完美工作.
//
// 关键词: backward compat canonical bracket
func TestRawExtract_BackwardCompat_CanonicalBracket(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `[tool_call name=bash]{"command":"ls -la"}[/tool_call]`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Len(t, r.toolCalls, 1)
	tc := r.toolCalls[0]
	assert.Equal(t, "bash", tc.Function.Name)
	assert.Equal(t, `{"command":"ls -la"}`, tc.Function.Arguments,
		"canonical 路径 args 原文就是 JSON, 输出也保持 JSON")
}

// canonical angle: 标准的 <tool_call name="X">JSON</tool_call> 兼容.
//
// 关键词: backward compat canonical angle
func TestRawExtract_BackwardCompat_CanonicalAngle(t *testing.T) {
	r := newRec()
	e := newExtractorWithRec(r)
	body := `<tool_call name="bash">{"command":"pwd"}</tool_call>`
	require.NoError(t, e.Write([]byte(body)))
	require.NoError(t, e.Flush())
	require.Len(t, r.toolCalls, 1)
	tc := r.toolCalls[0]
	assert.Equal(t, "bash", tc.Function.Name)
	assert.Equal(t, `{"command":"pwd"}`, tc.Function.Arguments)
}
