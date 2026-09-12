package loop_default

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestCompletionCheckpointRearmsAfterDiscoveredWork(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	responses := []string{
		`{"@action":"observe","identifier":"inventory"}`,
		`{"@action":"finish","identifier":"premature_finish"}`,
		`{"@action":"observe","identifier":"verify_second","todo_delta":{"add":[{"id":"second","text":"Verify the second configuration revealed by inventory; check its actual value"}],"current":"second"}}`,
		`{"@action":"finish","identifier":"close_second","todo_delta":{"close":[{"id":"second","outcome":"resolved","reason":"second value confirmed by observation-2","refs":["observation-2"]}]}}`,
		testReviewedFinish,
	}
	invoker := newPostIterationTestInvoker(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		index := len(prompts)
		prompts = append(prompts, req.GetPrompt())
		require.Less(t, index, len(responses), "unchanged reviewed work must finish without another loop")
		rsp := cfg.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(responses[index]))
		rsp.Close()
		return rsp, nil
	})
	observations := 0
	loop := newPostIterationTestLoop(t, invoker, reactloops.WithMaxIterations(4),
		reactloops.WithRegisterLoopAction("observe", "Read a configuration", nil, nil,
			func(loop *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				observations++
				if observations == 1 {
					op.Feedback("observation-1: primary is verified, inventory also names second.json, whose effective value is still unknown")
				} else {
					open, current, _ := loop.GetConfig().SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(loop.GetCurrentTask()))
					require.Len(t, open, 1, "discovery must be registered before its tool executes")
					require.Equal(t, "second", current)
					op.Feedback("observation-2: second.json effective value is 42")
				}
				op.Continue()
			}),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("discovered-work", ctx, "Verify both configurations"))
	require.Equal(t, 2, observations)
	require.Len(t, prompts, 5)
	require.Contains(t, prompts[2], "[COMPLETION REVIEW REQUIRED]")
	require.Contains(t, prompts[4], "[COMPLETION REVIEW REQUIRED]")
	require.Len(t, invoker.timelineValues("[COMPLETION_REVIEW_REQUIRED]"), 2)
	open, _, closed := loop.GetConfig().SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(loop.GetCurrentTask()))
	require.Empty(t, open)
	require.Len(t, closed, 1)
	require.Equal(t, []string{"observation-2"}, closed[0].Refs)
}

func TestMalformedFinishTerminatesWithBoundedValidationError(t *testing.T) {
	for _, tc := range []struct {
		name        string
		response    string
		wantError   string
		wantCalls   int
		checkpoints int
	}{
		{"missing review", `{"@action":"finish"}`, "completion_review.goal_evidence", 3, 1},
		{"invalid TODO sidecar", `{"@action":"finish","todo_delta":{"update":[{"id":"never-added","text":"pretend progress"}]}}`, "cannot finish with invalid TODO maintenance", 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			invoker := newPostIterationTestInvoker(func(cfg aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				mu.Lock()
				calls++
				mu.Unlock()
				rsp := cfg.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(tc.response))
				rsp.Close()
				return rsp, nil
			})
			invoker.GetConfig().(*aicommon.Config).AiTransactionAutoRetry = 2
			loop := newPostIterationTestLoop(t, invoker, reactloops.WithMaxIterations(4))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := loop.Execute("invalid-finish", ctx, "Verify the release configuration")
			require.ErrorContains(t, err, tc.wantError)
			require.NoError(t, ctx.Err(), "repeated malformed finish must fail validation before timeout")
			mu.Lock()
			require.Equal(t, tc.wantCalls, calls)
			mu.Unlock()
			require.Len(t, invoker.timelineValues("[COMPLETION_REVIEW_REQUIRED]"), tc.checkpoints)
			require.Empty(t, invoker.timelineValues("finish"))
		})
	}
}
