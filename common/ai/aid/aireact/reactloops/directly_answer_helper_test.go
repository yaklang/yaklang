package reactloops

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// newMinimalLoopForHelperTest 构造一个仅装备 vars 状态的 ReActLoop, 满足
// WrapDirectlyAnswerError / ActionVerifier 直接读 loop.Get("last_ai_decision_nonce")
// 的最小依赖. 不走 CreateLoopByName 的完整 invoker 链路.
// 关键词: directly_answer helper test, minimal ReActLoop, vars-only
func newMinimalLoopForHelperTest() *ReActLoop {
	return &ReActLoop{
		vars: omap.NewEmptyOrderedMap[string, any](),
	}
}

// TestWrapDirectlyAnswerError_NilErrReturnsNil err==nil 时不应包装.
// 关键词: WrapDirectlyAnswerError nil err 直通
func TestWrapDirectlyAnswerError_NilErrReturnsNil(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	loop.Set("last_ai_decision_nonce", "abcd1234")

	got := WrapDirectlyAnswerError(loop, nil)
	assert.Nil(t, got, "nil err should pass through as nil")
}

// TestWrapDirectlyAnswerError_NoLoopFallback loop==nil 时给最小 hint, 不丢原 err.
// 关键词: WrapDirectlyAnswerError nil loop fallback minimal hint
func TestWrapDirectlyAnswerError_NoLoopFallback(t *testing.T) {
	orig := utils.Error("inner")
	got := WrapDirectlyAnswerError(nil, orig)
	require.Error(t, got)
	assert.Contains(t, got.Error(), "inner",
		"原始错误信息必须保留")
	assert.Contains(t, got.Error(), "AITAG retry hint",
		"即便没 loop 也应附最小 hint, 让上层意识到这是 AITAG 引导路径")
}

// TestWrapDirectlyAnswerError_NoNonceFallback loop 不存 nonce 时退化最小 hint.
// 关键词: WrapDirectlyAnswerError nonce 缺失 fallback
func TestWrapDirectlyAnswerError_NoNonceFallback(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	// 不 Set last_ai_decision_nonce

	orig := utils.Error("answer_payload required")
	got := WrapDirectlyAnswerError(loop, orig)
	require.Error(t, got)
	assert.Contains(t, got.Error(), "answer_payload required",
		"原 ActionVerifier 错误必须保留")
	assert.Contains(t, got.Error(), "AITAG retry hint",
		"无 nonce 路径仍应附 hint 不丢失诊断")
	assert.Contains(t, got.Error(), "missing nonce",
		"无 nonce 路径必须明示, 方便定位 exec.go 没及时 set nonce")
	assert.NotContains(t, got.Error(), "<|FINAL_ANSWER_",
		"无 nonce 时不应输出占位空 nonce 的 AITAG 模板, 否则误导 AI")
}

// TestWrapDirectlyAnswerError_FullHint 有 nonce 时输出完整 nonce 化 AITAG 示例.
// 关键词: WrapDirectlyAnswerError 完整 hint, FINAL_ANSWER tag 模板
func TestWrapDirectlyAnswerError_FullHint(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	const nonce = "n0nC3-Xy7"
	loop.Set("last_ai_decision_nonce", nonce)

	orig := utils.Error("answer_payload is required for ActionDirectlyAnswer but empty")
	got := WrapDirectlyAnswerError(loop, orig)
	require.Error(t, got)
	msg := got.Error()
	assert.Contains(t, msg, "AITAG retry hint",
		"必须包含 hint 关键字, 让 RetryPromptBuilder 能识别")
	assert.Contains(t, msg, "<|FINAL_ANSWER_"+nonce+"|>",
		"必须出现 nonce 化的 FINAL_ANSWER 起始 tag, AI 才能照抄正确格式")
	assert.Contains(t, msg, "<|FINAL_ANSWER_END_"+nonce+"|>",
		"必须出现 nonce 化的 FINAL_ANSWER 结束 tag")
	assert.Contains(t, msg, "MUST emit AITAG block",
		"提示语必须强烈引导 AI 切到 AITAG, 而不是再次空 answer_payload")
	assert.Contains(t, msg, `{"@action":"directly_answer"}`,
		"示例里必须保留 directly_answer JSON 部分让 AI 能整段拷贝")
	assert.Contains(t, msg, "answer_payload is required for ActionDirectlyAnswer but empty",
		"原始 ActionVerifier 错误信息必须保留, 方便诊断")
}

// TestWrapDirectlyAnswerError_NonceTrim nonce 前后空白会被 trim 掉,
// 避免 AI 看到 "<|FINAL_ANSWER_  abc  |>" 这种带空格的标签照抄过来失效.
// 关键词: WrapDirectlyAnswerError nonce trim, 防 AI 把空白当 nonce
func TestWrapDirectlyAnswerError_NonceTrim(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	loop.Set("last_ai_decision_nonce", "   trimmed-nonce   \n")

	got := WrapDirectlyAnswerError(loop, utils.Error("x"))
	require.Error(t, got)
	msg := got.Error()
	assert.Contains(t, msg, "<|FINAL_ANSWER_trimmed-nonce|>",
		"nonce 前后空白必须 trim, 否则 AI 照抄就废了")
	assert.NotContains(t, msg, "<|FINAL_ANSWER_   trimmed",
		"trim 失败会让 AI 看到含空白的 tag 模板")
}

// TestActionBuiltinDirectlyAnswerVerifier_EmptyPayloadEmitsAITAGHint
// 端到端验证 reactloops 内置 directly_answer ActionVerifier 在
// answer_payload 与 FINAL_ANSWER tag 都缺失时, 抛出的错误已经被
// WrapDirectlyAnswerError 升级为带 nonce 的 AITAG hint, 让下一轮
// RetryPromptBuilder 把 hint 注入 prompt, 避免 5 次重试黑洞.
// 关键词: action_buildin directly_answer ActionVerifier AITAG hint, 5 次重试黑洞修复
func TestActionBuiltinDirectlyAnswerVerifier_EmptyPayloadEmitsAITAGHint(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	const nonce = "exec-set-nonce"
	loop.Set("last_ai_decision_nonce", nonce)

	// 模拟 AI 第一次只发 {"@action":"directly_answer"}, 既无 answer_payload
	// 也无 FINAL_ANSWER tag (loop.Get("tag_final_answer") 同样为空).
	action, err := aicommon.ExtractAction(`{"@action":"directly_answer"}`, "directly_answer")
	require.NoError(t, err)

	verr := loopAction_DirectlyAnswer.ActionVerifier(loop, action)
	require.Error(t, verr, "空 payload 必须报错, 否则下游会 emit 空答案")
	msg := verr.Error()
	assert.Contains(t, msg, "AITAG retry hint",
		"reactloops 内置 verifier 必须经由 WrapDirectlyAnswerError 注入 hint")
	assert.Contains(t, msg, "<|FINAL_ANSWER_"+nonce+"|>",
		"必须带当前 nonce 化的 AITAG 模板, AI 才能照抄修正")
	assert.Contains(t, msg, "answer_payload is required",
		"原 ActionVerifier 错误信息必须保留, 上游日志能定位根因")
}

// TestActionBuiltinDirectlyAnswerVerifier_HasPayloadPasses
// 反向验证: payload 非空时 ActionVerifier 直接通过, 不走 WrapDirectlyAnswerError.
// 关键词: action_buildin directly_answer ActionVerifier 正例, payload 非空跳过 hint
func TestActionBuiltinDirectlyAnswerVerifier_HasPayloadPasses(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	loop.Set("last_ai_decision_nonce", "should-not-be-used")

	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"hi"}`, "directly_answer")
	require.NoError(t, err)

	verr := loopAction_DirectlyAnswer.ActionVerifier(loop, action)
	require.NoError(t, verr, "answer_payload 非空时不应报错")
	got := loop.Get("directly_answer_payload")
	assert.Equal(t, "hi", strings.TrimSpace(got),
		"payload 应被透传到 directly_answer_payload, 让 ActionHandler 能直接 emit")
}
