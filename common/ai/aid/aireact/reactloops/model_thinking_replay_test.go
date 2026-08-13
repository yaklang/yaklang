package reactloops

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	aicommonmock "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type replayCapturingInvoker struct {
	*aicommonmock.MockInvoker
	displayEntry     string
	displayContent   string
	promptProjection string
	legacyCalls      int
}

func (i *replayCapturingInvoker) AddToTimeline(entry, content string) {
	i.displayEntry = entry
	i.displayContent = content
	i.legacyCalls++
}

func (i *replayCapturingInvoker) AddToTimelineWithPromptProjection(entry, displayContent, promptContent string) {
	i.displayEntry = entry
	i.displayContent = displayContent
	i.promptProjection = promptContent
}

func TestBuildModelThinkingReplayProjection(t *testing.T) {
	projection, err := buildModelThinkingReplayProjection(
		"turn_bad-nonce_1",
		" reasoning body ",
		` {"@action":"finish"} `,
	)
	require.NoError(t, err)
	require.Contains(t, projection, "<|TIMELINE_MODEL_THINKING_turnbadnonce1|>")
	result, err := aitag.SplitViaTAG(projection, timelineModelThinkingReplayTagName)
	require.NoError(t, err)
	require.Len(t, result.GetTaggedBlocks(), 1)
	var replay modelThinkingReplayRecord
	require.NoError(t, json.Unmarshal([]byte(result.GetTaggedBlocks()[0].Content), &replay))
	require.Equal(t, "reasoning body", replay.ReasoningContent)
	require.Equal(t, ` {"@action":"finish"} `, replay.Content)
}

func TestRecordModelThinkingTimelineReplaysOnlyCompleteSuccessfulRecord(t *testing.T) {
	invoker := &replayCapturingInvoker{MockInvoker: aicommonmock.NewMockInvoker(context.Background())}
	loop := &ReActLoop{invoker: invoker}

	loop.recordModelThinkingTimeline("successful reasoning", `{"@action":"finish"}`, "n1", true)
	require.Equal(t, TimelineEntryModelThinking, invoker.displayEntry)
	require.Equal(t, "successful reasoning", invoker.displayContent)
	require.Contains(t, invoker.promptProjection, timelineModelThinkingReplayTagName)
	require.Zero(t, invoker.legacyCalls)

	invoker.promptProjection = ""
	loop.recordModelThinkingTimeline("failed reasoning", "", "n2", false)
	require.Equal(t, "failed reasoning", invoker.displayContent)
	require.Empty(t, invoker.promptProjection)
	require.Equal(t, 1, invoker.legacyCalls)

	invoker.promptProjection = ""
	loop.recordModelThinkingTimeline("reason without action", "", "n3", true)
	require.Empty(t, invoker.promptProjection)
	require.Equal(t, 2, invoker.legacyCalls)

	invoker.promptProjection = ""
	loop.recordModelThinkingTimeline("   ", `{"@action":"finish"}`, "n4", true)
	require.Empty(t, invoker.promptProjection)
	require.True(t, strings.TrimSpace(invoker.displayContent) != "")
}

func TestBindDecisionResponseCaptureWaitsForCompleteOutputAndResetsReasoningPerAttempt(t *testing.T) {
	invoker := aicommonmock.NewMockInvoker(context.Background())
	config := invoker.GetConfig()
	loop := NewMinimalReActLoop(config, invoker)

	loop.appendModelThinkingChunk([]byte("rejected-attempt"))
	response := config.NewAIResponse()
	loop.bindDecisionResponseCapture(response)
	output := `{"@action":"finish","identifier":"complete"}`
	reader := response.GetOutputStreamReader("replay-capture-test", true, loop.emitter)
	response.EmitReasonStream(strings.NewReader("accepted-reasoning"))
	response.EmitOutputStream(strings.NewReader(output))
	response.Close()

	actual, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, output, string(actual))
	require.Eventually(t, func() bool {
		return loop.Get("last_ai_decision_response") == output
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, "accepted-reasoning", loop.takeModelThinkingForTimeline())
}
