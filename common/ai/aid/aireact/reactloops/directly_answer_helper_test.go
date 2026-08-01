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

func newMinimalLoopForHelperTest() *ReActLoop {
	return &ReActLoop{vars: omap.NewEmptyOrderedMap[string, any]()}
}

func TestWrapDirectlyAnswerError_NilErrReturnsNil(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	loop.Set("last_ai_decision_nonce", "abcd1234")
	assert.Nil(t, WrapDirectlyAnswerError(loop, nil))
}

func TestWrapDirectlyAnswerError_NoLoopFallback(t *testing.T) {
	got := WrapDirectlyAnswerError(nil, utils.Error("inner"))
	require.Error(t, got)
	assert.Contains(t, got.Error(), "inner")
	assert.Contains(t, got.Error(), "AITAG retry hint")
}

func TestWrapDirectlyAnswerError_NoNonceFallback(t *testing.T) {
	got := WrapDirectlyAnswerError(newMinimalLoopForHelperTest(), utils.Error("answer_payload required"))
	require.Error(t, got)
	assert.Contains(t, got.Error(), "answer_payload required")
	assert.Contains(t, got.Error(), "AITAG retry hint")
	assert.Contains(t, got.Error(), "missing nonce")
	assert.NotContains(t, got.Error(), "<|FINAL_ANSWER_")
}

func TestWrapDirectlyAnswerError_FullHint(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	const nonce = "n0nC3-Xy7"
	loop.Set("last_ai_decision_nonce", nonce)

	got := WrapDirectlyAnswerError(loop, utils.Error("answer_payload is required for ActionDirectlyAnswer but empty"))
	require.Error(t, got)
	msg := got.Error()
	assert.Contains(t, msg, "AITAG retry hint")
	assert.Contains(t, msg, "<|FINAL_ANSWER_"+nonce+"|>")
	assert.Contains(t, msg, "<|FINAL_ANSWER_END_"+nonce+"|>")
	assert.Contains(t, msg, "MUST emit AITAG block")
	assert.Contains(t, msg, `{"@action":"directly_answer"}`)
	assert.Contains(t, msg, "answer_payload is required for ActionDirectlyAnswer but empty")
}

func TestWrapDirectlyAnswerError_NonceTrim(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	loop.Set("last_ai_decision_nonce", "   trimmed-nonce   \n")

	got := WrapDirectlyAnswerError(loop, utils.Error("x"))
	require.Error(t, got)
	msg := got.Error()
	assert.Contains(t, msg, "<|FINAL_ANSWER_trimmed-nonce|>")
	assert.NotContains(t, msg, "<|FINAL_ANSWER_   trimmed")
}

func TestActionBuiltinDirectlyAnswerVerifier_EmptyPayloadEmitsAITAGHint(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	const nonce = "exec-set-nonce"
	loop.Set("last_ai_decision_nonce", nonce)

	action, err := aicommon.ExtractAction(`{"@action":"directly_answer"}`, "directly_answer")
	require.NoError(t, err)
	verr := loopAction_DirectlyAnswer.ActionVerifier(loop, action)
	require.Error(t, verr)
	assert.Contains(t, verr.Error(), "AITAG retry hint")
	assert.Contains(t, verr.Error(), "<|FINAL_ANSWER_"+nonce+"|>")
	assert.Contains(t, verr.Error(), "answer_payload is required")
}

func TestActionBuiltinDirectlyAnswerVerifier_HasPayloadPasses(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	loop.Set("last_ai_decision_nonce", "should-not-be-used")

	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"hi"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, action))
	assert.Equal(t, "hi", strings.TrimSpace(loop.Get("directly_answer_payload")))
}
