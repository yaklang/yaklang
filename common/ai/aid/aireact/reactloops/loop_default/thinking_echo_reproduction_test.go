package loop_default_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

const reproducedPromptExampleReasoning = `The output examples are:
{"@action":"directly_answer","answer_payload":"...[your-answer not a markdown].."}
{"@action":"directly_answer"}
<|FINAL_ANSWER_CURRENT_NONCE|>
# markdown in mass
<|FINAL_ANSWER_END_CURRENT_NONCE|>`

const reproducedPromptExampleEcho = `{"@action":"directly_answer","answer_payload":"...[your-answer not a markdown].."}
{"@action":"directly_answer"}
<|FINAL_ANSWER_CURRENT_NONCE|>
# markdown in mass
<|FINAL_ANSWER_END_CURRENT_NONCE|>`

// TestDefaultLoop_ReproducesReasoningReplayPromptExampleEcho characterizes the
// complete server-side chain behind the UI symptom:
//
//  1. an accepted iteration returns protocol examples in native reasoning;
//  2. the next main-decision prompt replays that reasoning;
//  3. a model echo of the two examples is accepted as a real directly_answer;
//  4. both answer_payload and FINAL_ANSWER are streamed before verification,
//     while the default verifier commits answer_payload and ignores the tag.
//
// The second scripted response is conditional on observing the replay in its
// request, so the test cannot pass by merely emitting the bad output directly.
func TestDefaultLoop_ReproducesReasoningReplayPromptExampleEcho(t *testing.T) {
	var (
		mu      sync.Mutex
		prompts []string
		events  []*schema.AiOutputEvent
		calls   int
	)

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil {
				return
			}
			copyEvent := *event
			copyEvent.StreamDelta = append([]byte(nil), event.StreamDelta...)
			copyEvent.Content = append([]byte(nil), event.Content...)
			mu.Lock()
			events = append(events, &copyEvent)
			mu.Unlock()
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			mu.Lock()
			callIndex := calls
			calls++
			prompts = append(prompts, request.GetPrompt())
			mu.Unlock()

			response := config.NewAIResponse()
			switch callIndex {
			case 0:
				response.EmitReasonStream(strings.NewReader(reproducedPromptExampleReasoning))
				response.EmitOutputStream(strings.NewReader(`{"@action":"record_note","note":"continue"}`))
			case 1:
				prompt := request.GetPrompt()
				if strings.Contains(prompt, "TIMELINE_MODEL_THINKING_V1_") &&
					strings.Contains(prompt, "...[your-answer not a markdown]..") &&
					strings.Contains(prompt, "FINAL_ANSWER_CURRENT_NONCE") {
					response.EmitOutputStream(strings.NewReader(reproducedPromptExampleEcho))
				} else {
					response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
				}
			default:
				response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
			}
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		reactIns,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithRegisterLoopAction(
			"record_note",
			"record a test note and continue",
			nil,
			nil,
			func(_ *reactloops.ReActLoop, _ *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				operator.Continue()
			},
		),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("reasoning-echo-reproduction", ctx, "continue the task"))
	loop.GetEmitter().WaitForStream()

	mu.Lock()
	capturedPrompts := append([]string(nil), prompts...)
	capturedEvents := append([]*schema.AiOutputEvent(nil), events...)
	callCount := calls
	mu.Unlock()

	require.GreaterOrEqual(t, callCount, 4, "direct answer must be followed by the two-step finish checkpoint")
	require.GreaterOrEqual(t, len(capturedPrompts), 2)
	require.Equal(t, 1, strings.Count(capturedPrompts[0], "...[your-answer not a markdown].."),
		"the initial prompt already contains one executable copy in output_example")
	require.Contains(t, capturedPrompts[1], "TIMELINE_MODEL_THINKING_V1_")
	require.GreaterOrEqual(t, strings.Count(capturedPrompts[1], "...[your-answer not a markdown].."), 2,
		"reasoning replay adds another copy beside the always-present output_example")
	require.Contains(t, capturedPrompts[1], "FINAL_ANSWER_CURRENT_NONCE")

	answerStreamStarts, answerStream, answerSources := snapshotAnswerStreams(capturedEvents)

	require.Equal(t, 2, answerStreamStarts,
		"one copied response opens both answer_payload and FINAL_ANSWER streams before verification")
	require.Contains(t, answerStream, "...[your-answer not a markdown]..")
	require.Contains(t, answerStream, "# markdown in mass")
	require.Contains(t, answerSources, "answer_payload")

	require.Equal(t, "...[your-answer not a markdown]..", strings.TrimSpace(loop.Get("directly_answer_payload")),
		"the default verifier commits answer_payload and ignores the simultaneously parsed FINAL_ANSWER")
	require.Equal(t, "# markdown in mass", strings.TrimSpace(loop.Get("tag_final_answer")))
}

// TestDefaultLoop_PromptExampleEchoDoesNotRequireReasoningReplay is the control
// case. The first request already contains the executable output_example, so a
// direct model copy produces the same two user-visible streams without any
// prior reasoning history. This proves replay is an amplifier, not a necessary
// precondition for the parser/UI symptom.
func TestDefaultLoop_PromptExampleEchoDoesNotRequireReasoningReplay(t *testing.T) {
	var (
		mu     sync.Mutex
		events []*schema.AiOutputEvent
		calls  int
	)

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil {
				return
			}
			copyEvent := *event
			copyEvent.StreamDelta = append([]byte(nil), event.StreamDelta...)
			copyEvent.Content = append([]byte(nil), event.Content...)
			mu.Lock()
			events = append(events, &copyEvent)
			mu.Unlock()
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			mu.Lock()
			callIndex := calls
			calls++
			mu.Unlock()

			response := config.NewAIResponse()
			if callIndex == 0 {
				require.NotContains(t, request.GetPrompt(), "TIMELINE_MODEL_THINKING_V1_")
				require.Contains(t, request.GetPrompt(), "...[your-answer not a markdown]..")
				require.Contains(t, request.GetPrompt(), "FINAL_ANSWER_CURRENT_NONCE")
				response.EmitOutputStream(strings.NewReader(reproducedPromptExampleEcho))
			} else {
				response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
			}
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		reactIns,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("prompt-example-echo-control", ctx, "continue the task"))
	loop.GetEmitter().WaitForStream()

	mu.Lock()
	capturedEvents := append([]*schema.AiOutputEvent(nil), events...)
	callCount := calls
	mu.Unlock()
	require.GreaterOrEqual(t, callCount, 3)

	answerStreamStarts, answerStream, _ := snapshotAnswerStreams(capturedEvents)
	require.Equal(t, 2, answerStreamStarts)
	require.Contains(t, answerStream, "...[your-answer not a markdown]..")
	require.Contains(t, answerStream, "# markdown in mass")
	require.Equal(t, "...[your-answer not a markdown]..", strings.TrimSpace(loop.Get("directly_answer_payload")))
	require.Equal(t, "# markdown in mass", strings.TrimSpace(loop.Get("tag_final_answer")))
}

// TestDefaultLoop_ReasonStreamProtocolTextIsNotParsedAsAnswer verifies the
// direct-stream boundary: protocol-looking bytes delivered only through the
// provider reasoning channel are rendered as thought data, but never enter the
// action/AITag parser that consumes the normal output channel.
func TestDefaultLoop_ReasonStreamProtocolTextIsNotParsedAsAnswer(t *testing.T) {
	var (
		mu     sync.Mutex
		events []*schema.AiOutputEvent
		calls  int
	)

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil {
				return
			}
			copyEvent := *event
			copyEvent.StreamDelta = append([]byte(nil), event.StreamDelta...)
			copyEvent.Content = append([]byte(nil), event.Content...)
			mu.Lock()
			events = append(events, &copyEvent)
			mu.Unlock()
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			mu.Lock()
			callIndex := calls
			calls++
			mu.Unlock()

			response := config.NewAIResponse()
			if callIndex == 0 {
				response.EmitReasonStream(strings.NewReader(reproducedPromptExampleReasoning))
			}
			response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		reactIns,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("reason-stream-boundary", ctx, "finish the task"))
	loop.GetEmitter().WaitForStream()

	mu.Lock()
	capturedEvents := append([]*schema.AiOutputEvent(nil), events...)
	mu.Unlock()

	answerStreamStarts, answerStream, _ := snapshotAnswerStreams(capturedEvents)
	require.Zero(t, answerStreamStarts)
	require.Empty(t, answerStream)

	var thoughtStream bytes.Buffer
	for _, event := range capturedEvents {
		if event != nil && event.Type == schema.EVENT_TYPE_STREAM && event.NodeId == "re-act-loop-thought" {
			thoughtStream.Write(event.StreamDelta)
		}
	}
	require.Contains(t, thoughtStream.String(), "...[your-answer not a markdown]..")
	require.Contains(t, thoughtStream.String(), "# markdown in mass")
}

func snapshotAnswerStreams(events []*schema.AiOutputEvent) (int, string, []string) {
	starts := 0
	var stream bytes.Buffer
	var sources []string
	for _, event := range events {
		if event == nil || event.NodeId != "re-act-loop-answer-payload" {
			continue
		}
		if event.Type == schema.EVENT_TYPE_STREAM_START {
			starts++
			sources = append(sources, event.VizSource)
		}
		if event.Type == schema.EVENT_TYPE_STREAM {
			stream.Write(event.StreamDelta)
		}
	}
	return starts, stream.String(), sources
}
