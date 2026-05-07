package loop_plan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// TestExtractFactsAITagFromRawResponse_TurnNonce 验证使用真实 turn nonce 时
// FACTS AITag 块能被正确提取出来.
//
// 关键词: extractFactsAITagFromRawResponse turn nonce 用例
func TestExtractFactsAITagFromRawResponse_TurnNonce(t *testing.T) {
	const nonce = "abc1"
	raw := `{"@action":"output_facts"}
<|FACTS_` + nonce + `|>
## 目标主机
- IP: 10.0.0.50
<|FACTS_END_` + nonce + `|>
`
	got := extractFactsAITagFromRawResponse(raw)
	assert.Contains(t, got, "## 目标主机", "应该提取到 FACTS section 标题")
	assert.Contains(t, got, "10.0.0.50", "应该提取到 bullet 内容")
}

// TestExtractFactsAITagFromRawResponse_LiteralCurrentNonce 是这次修复的核心
// 用例: AI 把 prompt 示例里的 `CURRENT_NONCE` 占位符当作字面量直接照抄, 兜底
// 路径仍然能把 FACTS 块抠出来. 这正是本次截图里观察到的实际故障形态.
//
// 关键词: extractFactsAITagFromRawResponse CURRENT_NONCE 字面量兼容,
//
//	output_facts 5 次重试黑洞修复
func TestExtractFactsAITagFromRawResponse_LiteralCurrentNonce(t *testing.T) {
	raw := `{"@action": "output_facts"}
` + "```markdown\n" + `<|FACTS_CURRENT_NONCE|>
## 目标
- id.redhaze.top: 198.18.0.53, 198.18.0.56
<|FACTS_END_CURRENT_NONCE|>
` + "```"
	got := extractFactsAITagFromRawResponse(raw)
	require.NotEmpty(t, got, "AI 用 CURRENT_NONCE 字面量输出时, 兜底必须能提取出 FACTS")
	assert.Contains(t, got, "## 目标")
	assert.Contains(t, got, "198.18.0.53")
}

// TestExtractFactsAITagFromRawResponse_MultipleBlocks 验证多个 FACTS 块会被
// 拼接返回, 不会只拿第一个就丢弃后面的.
//
// 关键词: extractFactsAITagFromRawResponse 多块拼接
func TestExtractFactsAITagFromRawResponse_MultipleBlocks(t *testing.T) {
	raw := `{"@action":"output_facts"}
<|FACTS_n1|>
## 块一
- a
<|FACTS_END_n1|>

<|FACTS_n2|>
## 块二
- b
<|FACTS_END_n2|>
`
	got := extractFactsAITagFromRawResponse(raw)
	assert.Contains(t, got, "## 块一")
	assert.Contains(t, got, "- a")
	assert.Contains(t, got, "## 块二")
	assert.Contains(t, got, "- b")
}

// TestExtractFactsAITagFromRawResponse_NoMatch 没有 FACTS 块时返回空字符串,
// 让上层兜底链路继续往下走.
//
// 关键词: extractFactsAITagFromRawResponse 无匹配返回空
func TestExtractFactsAITagFromRawResponse_NoMatch(t *testing.T) {
	raw := `{"@action":"output_facts"}
this response has no FACTS block at all
`
	got := extractFactsAITagFromRawResponse(raw)
	assert.Empty(t, got, "无 FACTS 块时必须返回空, 让 handler 继续走 autoGenerateFacts 兜底")
}

// TestExtractFactsAITagFromRawResponse_EmptyContent 空输入直接返回空,
// 不应 panic 或卡住.
//
// 关键词: extractFactsAITagFromRawResponse 空输入边界
func TestExtractFactsAITagFromRawResponse_EmptyContent(t *testing.T) {
	assert.Empty(t, extractFactsAITagFromRawResponse(""))
	assert.Empty(t, extractFactsAITagFromRawResponse("   \n  \n"))
}

// TestVerifyOutputFactsAction_AcceptsEmptyFacts 是 UX 修复的关键验证: AI 选了
// {"@action":"output_facts"} 但 facts 字段为空时, verifier 不应再拒绝整个
// AI 事务. 这个改动直接消除了 5 次重试黑洞 + [AI Transaction Failed] 致命中断.
//
// 关键词: verifyOutputFactsAction 容错, 尊重 JSON action, 避免 5 次重试黑洞
func TestVerifyOutputFactsAction_AcceptsEmptyFacts(t *testing.T) {
	emptyAction, err := aicommon.ExtractAction(`{"@action":"output_facts"}`, "output_facts")
	require.NoError(t, err)

	// loop 传 nil 也必须容错通过, 因为 verifier 唯一职责就是"尊重 JSON 不拒绝".
	verr := verifyOutputFactsAction(nil, emptyAction)
	assert.NoError(t, verr,
		"verifier 必须对空 facts 容错, 让 handler 路径继续兜底而不是把整个 AI 事务搞挂")
}

// TestVerifyOutputFactsAction_AcceptsActionWithFacts 反向用例: facts 字段
// 完整时 verifier 同样通过, 不引入新的拒绝路径.
//
// 关键词: verifyOutputFactsAction 完整 facts 通过
func TestVerifyOutputFactsAction_AcceptsActionWithFacts(t *testing.T) {
	action, err := aicommon.ExtractAction(`{"@action":"output_facts","facts":"## ok\n- v1"}`, "output_facts")
	require.NoError(t, err)

	verr := verifyOutputFactsAction(nil, action)
	assert.NoError(t, verr)
}

// TestResolveOutputFactsContent_PreferActionField 主路径: action.params[facts]
// 已经有内容时直接返回, 不走 raw response 兜底.
//
// 关键词: resolveOutputFactsContent 主路径
func TestResolveOutputFactsContent_PreferActionField(t *testing.T) {
	const factsBody = "## 主路径\n- v1"
	escaped := strings.ReplaceAll(factsBody, "\n", `\n`)
	action, err := aicommon.ExtractAction(`{"@action":"output_facts","facts":"`+escaped+`"}`, "output_facts")
	require.NoError(t, err)

	got := resolveOutputFactsContent(nil, action)
	assert.Contains(t, got, "## 主路径")
	assert.Contains(t, got, "- v1")
}
