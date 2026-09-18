package aicommon

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimelineAuxiliaryRefineUsesConfigSpeedAndFallback(t *testing.T) {
	registerTimelineTestLiteForge(t)
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			calls := 0
			cfg := NewTestConfig(ctx, WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true),
				WithAIAutoRetry(1), WithAITransactionAutoRetry(1),
				WithSpeedPriorityAICallback(func(_ AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
					calls++
					require.Equal(t, CallerLabelTimelineHeadRefine, request.GetCallerLabel())
					require.Equal(t, 1, strings.Count(request.GetPrompt(), `"required": ["@action", "key_findings", "open_failures"]`))
					require.NotContains(t, request.GetPrompt(), "UNRELATED_TIMELINE")
					if fail {
						return nil, errors.New("reducer unavailable")
					}
					response := NewUnboundAIResponse()
					response.EmitOutputStream(strings.NewReader(`{"@action":"timeline-reducer","key_findings":["preserved finding"],"open_failures":"still blocked"}`))
					response.Close()
					return response, nil
				}),
			)
			// A legacy caller must never override the bound Config's Speed route.
			legacyCalls := 0
			timeline := NewTimeline(&ProxyAICaller{callFunc: func(*AIRequest) (*AIResponse, error) {
				legacyCalls++
				return nil, errors.New("legacy caller must not run")
			}}, nil)
			timeline.SoftBindConfig(cfg, nil)
			timeline.PushText(1, "UNRELATED_TIMELINE")
			head := strings.Repeat("old completed work ", 500)
			got := timeline.refineCompressedHeadLocked(head, 100, "test-nonce")
			require.Equal(t, 1, calls)
			require.Zero(t, legacyCalls)
			if fail {
				require.Equal(t, enforceOutputTokenBudget(head, 100), got)
			} else {
				require.Contains(t, got, "preserved finding")
				require.Contains(t, got, "still blocked")
			}
		})
	}
}

func TestTimelineAuxiliaryBatchFailureDoesNotDeleteItems(t *testing.T) {
	registerTimelineTestLiteForge(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	calls := 0
	cfg := NewTestConfig(ctx, WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true),
		WithAIAutoRetry(1), WithAITransactionAutoRetry(1),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			calls++
			require.Equal(t, CallerLabelTimelineBatchCompress, request.GetCallerLabel())
			return nil, errors.New("reducer unavailable")
		}),
	)
	timeline := compressionBenchmarkFixture(cfg)
	items := timeline.idToTimelineItem.Values()
	timeline.batchCompressOldestWithRecent(items[:50], items[50:])
	require.Equal(t, 1, calls)
	require.Nil(t, timeline.compressedHead)
	require.Len(t, timeline.getActiveTimelineItemIDs(), 60)
}

// The host-state unit tests cannot import aiforge from package aicommon.
// Register a scoped adapter for reducers only, and restore the prior callback.
// The real renderer, streaming callbacks and lifecycle are exercised separately
// in aiforge/liteforge_auxiliary_stream_test.go.
func registerTimelineTestLiteForge(t testing.TB) {
	t.Helper()
	previous := liteforgeExecuteFunc
	RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*ForgeResult, error) {
		var req *LiteForgeInvokeRequest
		var configOpts []ConfigOption
		for _, opt := range opts {
			switch value := opt.(type) {
			case *LiteForgeInvokeRequest:
				req = value
			case ConfigOption:
				configOpts = append(configOpts, value)
			}
		}
		if req == nil || (req.ActionName != CallerLabelTimelineBatchCompress && req.ActionName != CallerLabelTimelineHeadRefine) {
			if previous != nil {
				return previous(prompt, opts...)
			}
			return nil, errors.New("no test LiteForge adapter for this task")
		}
		cfg := NewConfig(req.Context, append([]ConfigOption{WithDisableAutoSkills(true)}, configOpts...)...)
		request := NewAIRequest(prompt+"\n"+req.OutputSchema,
			NewGeneralKVConfig(req.Options...).GetExtraRequestOpts()...)
		response, err := cfg.CallAI(request)
		if err != nil {
			return nil, err
		}
		action, err := ExtractValidActionFromStream(req.Context, response.GetUnboundStreamReader(false), req.OutputActionName)
		if err != nil {
			return nil, err
		}
		return &ForgeResult{Action: action}, nil
	})
	t.Cleanup(func() { RegisterLiteForgeExecuteCallback(previous) })
}
