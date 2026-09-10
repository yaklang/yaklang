package reactloops

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"
)

type platformAnswerTestConfig struct{ *softCheckpointConfig }

func (*platformAnswerTestConfig) GetFinishAfterDirectlyAnswer() bool { return true }
func TestPlatformAnswerStillHonorsRemainingWork(t *testing.T) {
	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"answer"}`, "directly_answer")
	require.NoError(t, err)
	loop, _, cfg, _ := newTodoGateTestLoop(t, nil)
	require.False(t, ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop, action))
	loop.config = &platformAnswerTestConfig{cfg}
	require.True(t, ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop, action))
	withDelta, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"progress","todo_delta":{"add":[{"id":"next","text":"remaining work"}],"current":"next"}}`, "directly_answer")
	require.NoError(t, err)
	require.False(t, ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop, withDelta))
	cfg.active = []aicommon.VerificationTodoItem{{ID: "open", Status: aicommon.VerificationTodoStatusPending}}
	require.False(t, ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop, action))
}
