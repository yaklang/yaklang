package aicommon

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimelineCompressionSummaryDoesNotWaitForMemoryStream(t *testing.T) {
	for _, mode := range []string{"memory", "failed-tail", "changed-selection"} {
		t.Run(mode, func(t *testing.T) {
			registerTimelineTestLiteForge(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			reader, writer := io.Pipe()
			t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
			cfg := NewTestConfig(ctx, WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true),
				WithAIAutoRetry(1), WithAITransactionAutoRetry(1),
				WithSpeedPriorityAICallback(func(_ AICallerConfigIf, _ *AIRequest) (*AIResponse, error) {
					response := NewUnboundAIResponse()
					response.EmitOutputStream(reader)
					response.Close()
					return response, nil
				}))
			tl := cfg.Timeline
			tl.PushText(1, "original needed for later verification")
			summaries := make(chan TimelineSummaryEvent, 1)
			memory := make(chan TimelineMemoryEvent, 1)
			tl.RegisterSummaryCallback("next-loop", func(event TimelineSummaryEvent) { summaries <- event })
			tl.RegisterMemoryCallback("storage", func(event TimelineMemoryEvent) { memory <- event })
			type outcome struct {
				result *TimelineCompressionResult
				err    error
			}
			finished := make(chan outcome, 1)
			go func() { result, err := tl.CompressOnce(compressionTestOptions()); finished <- outcome{result, err} }()
			_, err := io.WriteString(writer, `{"@action":"timeline-summary","summary":"verified history; next step pending","ratain_timeline_item_range":"1","memory_entities":`)
			require.NoError(t, err)
			select {
			case done := <-finished:
				require.NoError(t, done.err)
				require.Equal(t, []int64{1}, done.result.RetainedIDs)
				require.Contains(t, RenderTimelineFrozenOpen(tl).Frozen, "original needed")
			case <-ctx.Done():
				t.Fatal("summary commit waited for the memory tail")
			}
			select {
			case event := <-summaries:
				require.Equal(t, "1", event.RetainedRange)
				require.Equal(t, []int64{1}, event.RetainedIDs)
			case <-ctx.Done():
				t.Fatal("missing summary notification")
			}
			select {
			case <-memory:
				t.Fatal("memory notified before its output was complete")
			default:
			}
			// The main loop can append new history while memory is still being generated.
			tl.PushText(2, "next step executed before memory arrived")
			if mode == "failed-tail" {
				require.NoError(t, writer.CloseWithError(io.ErrUnexpectedEOF))
			} else {
				raw, err := json.Marshal([]any{compressionMemoryFixture()})
				require.NoError(t, err)
				tail := string(raw)
				if mode == "changed-selection" {
					tail += `,"summary":"changed after publication"`
				}
				_, err = io.WriteString(writer, tail+"}")
				require.NoError(t, err)
				require.NoError(t, writer.Close())
			}
			select {
			case event := <-memory:
				if mode == "memory" {
					require.NoError(t, event.Err)
					require.Len(t, event.MemoryEntities, 1)
					require.Equal(t, event.MemoryEntities, tl.GetSessionMemoryCandidates())
				} else {
					require.Error(t, event.Err)
					require.Empty(t, event.MemoryEntities)
					require.Empty(t, tl.GetSessionMemoryCandidates())
				}
				require.EqualValues(t, 1, event.ThroughID)
			case <-ctx.Done():
				t.Fatal("missing memory completion")
			}
			require.Contains(t, RenderTimelineFrozenOpen(tl).Frozen, "verified history; next step pending")
			require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "next step executed")
		})
	}
}

func TestTimelineCompressionSlowMemoryConsumerDoesNotBlockSummary(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "verified observation")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { return compressionMockSummary("verified summary"), nil })
	started, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		close(release)
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("memory observer did not stop")
		}
	})
	tl.RegisterMemoryCallback("storage", func(event TimelineMemoryEvent) {
		close(started)
		<-release
		close(stopped)
	})
	result, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, "verified summary", result.Summary)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("memory observer did not start")
	}
	require.False(t, tl.compressing)
	timeline, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Nil(t, timeline)
}

func TestTimelineCompressionMemoryOnlySubscription(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "verified observation")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { return compressionMockSummary("verified summary"), nil })
	memory := make(chan TimelineMemoryEvent, 1)
	tl.RegisterMemoryCallback("replaced", func(TimelineMemoryEvent) { panic("superseded") })
	tl.RegisterMemoryCallback("replaced", func(event TimelineMemoryEvent) { memory <- event })
	tl.RegisterMemoryCallback("removed", func(TimelineMemoryEvent) { t.Error("unregistered observer") })
	tl.RegisterMemoryCallback("removed", nil)
	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	select {
	case event := <-memory:
		require.NoError(t, event.Err)
		require.Empty(t, event.MemoryEntities)
		require.True(t, strings.Contains(event.Prompt, "OPEN_TIMELINE_"))
	case <-time.After(5 * time.Second):
		t.Fatal("memory-only subscriber was not notified")
	}
}

func TestActionRootFieldsStayWithinOneObject(t *testing.T) {
	var observed []map[string]any
	action := NewActionMaker("timeline-summary", WithActionRootFieldsCallback(func(fields map[string]any) {
		observed = append(observed, fields)
	})).ReadFromReader(context.Background(), strings.NewReader(
		`{"summary":"first"} {"nested":{"summary":"hidden"},"ratain_timeline_item_range":"1"}`))
	require.NoError(t, action.WaitParseResult(context.Background()))
	for _, fields := range observed {
		if _, found := fields["ratain_timeline_item_range"]; found {
			require.NotContains(t, fields, "summary", "fields from separate objects must not form a selection")
		}
		require.NotEqual(t, "hidden", fields["summary"], "nested fields are not root fields")
	}
	require.NotEmpty(t, observed)
}
