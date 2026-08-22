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

func TestShouldAutoFinishAfterSimpleQueryDirectlyAnswer(t *testing.T) {
	loop, _, _, _ := newTodoGateTestLoop(t, nil)
	loop.Set("intent_hint", loopIntentHintSimpleQuery)
	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"你好"}`, "directly_answer")
	require.NoError(t, err)
	assert.True(t, ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop, action))

	loop.Set("intent_hint", "capabilities_matched")
	assert.False(t, ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop, action))

	loop.Set("intent_hint", loopIntentHintSimpleQuery)
	actionWithDelta, err := aicommon.ExtractAction(
		`{"@action":"directly_answer","answer_payload":"hi","todo_delta":{"add":[{"id":"follow_up","text":"继续处理已发现的问题"}],"current":"follow_up"}}`,
		"directly_answer",
	)
	require.NoError(t, err)
	assert.False(t, ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop, actionWithDelta))

	loopWithOpenTodo, _, _, _ := newTodoGateTestLoop(t, []aicommon.VerificationTodoItem{{ID: "still_open", Status: aicommon.VerificationTodoStatusPending}})
	loopWithOpenTodo.Set("intent_hint", loopIntentHintSimpleQuery)
	assert.False(t, ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loopWithOpenTodo, action))
}

func TestDirectlyAnswerContinueAutoFinishesSimpleQuery(t *testing.T) {
	loop, invoker, _, task := newTodoGateTestLoop(t, nil)
	loop.Set("intent_hint", loopIntentHintSimpleQuery)
	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"你好"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, action))

	op := NewActionHandlerOperator(task)
	loopAction_DirectlyAnswer.ActionHandler(loop, action, op)

	terminated, termErr := op.IsTerminated()
	require.True(t, terminated)
	require.NoError(t, termErr)
	require.False(t, op.IsContinued())
	assert.Contains(t, strings.Join(invoker.timeline, "\n"), "simple_query")
}

func TestRejectDuplicateDirectlyAnswerWithoutTodoDelta(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	first, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"report one"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, first))
	noteDirectlyAnswerDeliveredWithoutTodoDelta(loop, first)

	second, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"report two"}`, "directly_answer")
	require.NoError(t, err)
	verifierErr := loopAction_DirectlyAnswer.ActionVerifier(loop, second)
	require.Error(t, verifierErr)
	assert.Contains(t, verifierErr.Error(), "already delivered")
	assert.Contains(t, verifierErr.Error(), "finish")

	emptyDelta, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"report three","todo_delta":{}}`, "directly_answer")
	require.NoError(t, err)
	require.Error(t, loopAction_DirectlyAnswer.ActionVerifier(loop, emptyDelta), "empty todo_delta is omitted and must not bypass the duplicate guard")
}

func TestRejectDuplicateDirectlyAnswerAllowsAnswerWithTodoDelta(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	first, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"report one"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, first))
	noteDirectlyAnswerDeliveredWithoutTodoDelta(loop, first)

	withDelta, err := aicommon.ExtractAction(
		`{"@action":"directly_answer","answer_payload":"report two","todo_delta":{"add":[{"id":"follow","text":"继续验证新线索"}],"current":"follow"}}`,
		"directly_answer",
	)
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, withDelta))
}

func TestRejectDuplicateDirectlyAnswerAllowsLaterNoDeltaWhenFirstHadTodoDelta(t *testing.T) {
	loop := newMinimalLoopForHelperTest()
	first, err := aicommon.ExtractAction(
		`{"@action":"directly_answer","answer_payload":"report one","todo_delta":{"add":[{"id":"follow","text":"继续验证新线索"}],"current":"follow"}}`,
		"directly_answer",
	)
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, first))
	noteDirectlyAnswerDeliveredWithoutTodoDelta(loop, first)
	assert.False(t, directlyAnswerDeliveredWithoutTodoDelta(loop))

	second, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"report two"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, second))
}

func TestInvalidClosedTodoDeltaCannotBypassDuplicateDirectlyAnswer(t *testing.T) {
	loop, invoker, cfg, task := newTodoGateTestLoop(t, nil)
	setCurrentTodo(t, cfg, task, "deferred_old")
	results := cfg.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), &aicommon.TodoDelta{
		Close: []aicommon.TodoClose{{
			ID:      "deferred_old",
			Outcome: aicommon.TodoOutcomeDeferred,
			Reason:  "external prerequisite was unavailable",
		}},
	})
	require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))

	first, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"report one"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, first))
	noteDirectlyAnswerDeliveredWithoutTodoDelta(loop, first)

	second, err := aicommon.ExtractAction(
		`{"@action":"directly_answer","answer_payload":"report two","todo_delta":{"update":[{"id":"deferred_old","text":"resume it"}],"current":"deferred_old"}}`,
		"directly_answer",
	)
	require.NoError(t, err)
	validateTodoDeltaBeforeActionVerifier(loop, second)

	_, present := second.LookupCanonicalParam("todo_delta")
	require.False(t, present, "invalid delta must be removed before duplicate-answer verification")
	verifierErr := loopAction_DirectlyAnswer.ActionVerifier(loop, second)
	require.Error(t, verifierErr)
	require.Contains(t, verifierErr.Error(), "already delivered")
	timeline := strings.Join(invoker.timeline, "\n")
	require.Contains(t, timeline, "TODO_DELTA_ERROR")
	require.Contains(t, timeline, "use todo_delta.add with a new id")
}
