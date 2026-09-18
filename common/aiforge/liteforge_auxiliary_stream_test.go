package aiforge

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAuxiliaryLiteForgeResponseStreamAndLegacyCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	started := make(chan struct{})
	allowTail := make(chan struct{})
	streamDone := make(chan string, 1)
	legacyDone := make(chan string, 1)
	finished := make(chan struct{})
	var qualityCalls atomic.Int32
	var callbackErr error
	var resultText string
	var resultAfterStreams bool
	events := make(chan *schema.AiOutputEvent, 32)
	emitter := aicommon.NewEmitter("field-owner", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event.NodeId == "auxiliary-field" {
			events <- event
		}
		return event, nil
	})
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithQualityPriorityAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			qualityCalls.Add(1)
			return nil, errors.New("quality must not be called")
		}),
		aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			req.SetTaskIndex("owner-task")
			response := c.NewAIResponse()
			response.EmitOutputStream(reader)
			response.Close()
			return response, nil
		}),
	)
	go func() {
		_, _ = io.WriteString(writer, "{\"@action\":\"reducer-protocol\",\"text\":\"hello")
		select {
		case <-allowTail:
			_, _ = io.WriteString(writer, " world\",\"legacy\":\"compatible\"}")
		case <-ctx.Done():
		}
		writer.Close()
	}()
	go func() {
		defer close(finished)
		cfg.ScheduleAuxiliaryTask(ctx, "stream-task", func() string { return "summarize" },
			func(action *aicommon.Action) {
				resultText = action.GetString("text")
				resultAfterStreams = len(streamDone) == 1 && len(legacyDone) == 1
			},
			aicommon.WithAuxiliaryOutputSchema("reducer-protocol",
				"{\"type\":\"object\",\"properties\":{\"@action\":{\"const\":\"reducer-protocol\"},\"text\":{\"type\":\"string\"},\"legacy\":{\"type\":\"string\"}}}"),
			aicommon.WithAuxiliaryEmitter(emitter),
			aicommon.WithAuxiliaryOnError(func(err error) { callbackErr = err }),
			aicommon.WithAuxiliaryOpts(
				aicommon.WithLiteForgeDisableTimeline(),
				aicommon.WithGeneralConfigExtraRequestOpts(aicommon.WithAIRequest_DetachCheckpoint()),
				aicommon.WithGeneralConfigStreamableFieldResponseCallback([]string{"text"},
					func(_ string, field io.Reader, response *aicommon.AIResponse, bound *aicommon.Emitter) {
						close(started)
						data, _ := io.ReadAll(utils.JSONStringReader(field))
						streamDone <- response.GetTaskIndex() + ":" + string(data)
						bound.EmitDefaultSystemStreamEvent("auxiliary-field", strings.NewReader(string(data)), response.GetTaskIndex())
					}),
				aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{"legacy"},
					func(_ string, field io.Reader, _ *aicommon.Emitter) {
						data, _ := io.ReadAll(utils.JSONStringReader(field))
						legacyDone <- string(data)
					}),
			),
		)
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("field callback did not start before the full JSON arrived")
	}
	select {
	case <-finished:
		t.Fatal("result was delivered before the field stream completed")
	default:
	}
	close(allowTail)
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("auxiliary task did not finish")
	}
	require.NoError(t, callbackErr)
	require.True(t, resultAfterStreams)
	require.Equal(t, "hello world", resultText)
	require.Equal(t, "owner-task:hello world", <-streamDone)
	require.Equal(t, "compatible", <-legacyDone)
	require.Zero(t, qualityCalls.Load())
	emitter.WaitForStream()
	require.NotEmpty(t, events)
	for len(events) > 0 {
		event := <-events
		require.Equal(t, "owner-task", event.TaskIndex)
		require.Equal(t, "field-owner", event.CoordinatorId)
		require.True(t, event.IsSystem)
	}
}

func TestAuxiliaryLiteForgeErrorSkipAndCancellation(t *testing.T) {
	for _, mode := range []string{"malformed", "cancelled", "skip", "prompt-limit"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var calls, results, errorsSeen atomic.Int32
			var receivedErr error
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithFastAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					calls.Add(1)
					if mode == "cancelled" {
						cancel()
						return nil, req.GetContext().Err()
					}
					response := c.NewAIResponse()
					response.EmitOutputStream(strings.NewReader("{\"@action\":\"broken\",\"text\":"))
					response.Close()
					return response, nil
				}),
				aicommon.WithSingleAIModelMode(mode == "skip"),
			)
			name := "broken"
			if mode == "skip" {
				name = aicommon.CallerLabelToolCallReason
			}
			limit := 0
			if mode == "prompt-limit" {
				limit = 1
			}
			cfg.ScheduleAuxiliaryTask(ctx, name, func() string {
				require.NotEqual(t, "skip", mode, "skip must suppress prompt construction")
				return "summarize"
			}, func(*aicommon.Action) { results.Add(1) },
				aicommon.WithAuxiliaryOutputs(aitool.WithStringParam("text")),
				aicommon.WithAuxiliaryOnError(func(err error) { errorsSeen.Add(1); receivedErr = err }),
				aicommon.WithAuxiliaryOpts(aicommon.WithLiteForgeDisableTimeline(), aicommon.WithLiteForgeMaxPromptTokens(limit)),
			)
			require.Zero(t, results.Load())
			if mode == "skip" {
				require.Zero(t, errorsSeen.Load())
				require.Zero(t, calls.Load())
			} else {
				require.EqualValues(t, 1, errorsSeen.Load())
				require.Error(t, receivedErr)
				if mode == "prompt-limit" {
					require.Zero(t, calls.Load())
				} else {
					require.EqualValues(t, 1, calls.Load())
				}
			}
		})
	}
}
