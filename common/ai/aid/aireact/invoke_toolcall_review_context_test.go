package aireact

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestToolCallReviewSubAIHonorsCallContextCancellation(t *testing.T) {
	requests := make(chan *aicommon.AIRequest)
	tool, err := aitool.New(
		"review_context_tool",
		aitool.WithStringParam("input", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return "unused", nil
		}),
	)
	require.NoError(t, err)

	react, err := NewTestReAct(
		aicommon.WithTools(tool),
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			requests <- req
			ctx := req.GetContext()
			if ctx == nil {
				return nil, errors.New("review request has no call-scoped context")
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	)
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase(
		"review-context-task",
		"the explicit owning task query",
		context.Background(),
		react.Emitter,
		true,
	)
	decoyTask := aicommon.NewStatefulTaskBase(
		"unrelated-current-task",
		"query from an unrelated concurrent task",
		context.Background(),
		react.Emitter,
		true,
	)
	ownerLoop, err := reactloops.NewReActLoop(
		"review-context-owner-loop",
		react,
		reactloops.WithPersistentContextProvider(func(*reactloops.ReActLoop, string) (string, error) {
			return "OWNER_LOOP_INSTRUCTION", nil
		}),
		reactloops.WithOutputExampleContextProvider(func(*reactloops.ReActLoop, string) (string, error) {
			return "OWNER_LOOP_EXAMPLE", nil
		}),
	)
	require.NoError(t, err)
	decoyLoop, err := reactloops.NewReActLoop(
		"review-context-decoy-loop",
		react,
		reactloops.WithPersistentContextProvider(func(*reactloops.ReActLoop, string) (string, error) {
			return "DECOY_LOOP_INSTRUCTION", nil
		}),
		reactloops.WithOutputExampleContextProvider(func(*reactloops.ReActLoop, string) (string, error) {
			return "DECOY_LOOP_EXAMPLE", nil
		}),
	)
	require.NoError(t, err)
	task.SetReActLoop(ownerLoop)
	decoyTask.SetReActLoop(decoyLoop)
	// The review helpers below must use task, not this mutable session pointer.
	react.SetCurrentTask(decoyTask)
	paramPrompt, err := react.generateToolParamsPromptWithMetaForTask(task, tool, tool.Name)
	require.NoError(t, err)
	require.Contains(t, paramPrompt.Prompt, "the explicit owning task query")
	require.NotContains(t, paramPrompt.Prompt, "query from an unrelated concurrent task")
	require.Contains(t, paramPrompt.Prompt, "OWNER_LOOP_INSTRUCTION")
	require.Contains(t, paramPrompt.Prompt, "OWNER_LOOP_EXAMPLE")
	require.NotContains(t, paramPrompt.Prompt, "DECOY_LOOP_INSTRUCTION")
	require.NotContains(t, paramPrompt.Prompt, "DECOY_LOOP_EXAMPLE")

	tests := []struct {
		name                    string
		call                    func(context.Context) error
		expectExplicitTaskQuery bool
	}{
		{
			name:                    "wrong tool",
			expectExplicitTaskQuery: true,
			call: func(ctx context.Context) error {
				_, _, err := react._invokeToolCall_ReviewWrongToolForTask(ctx, task, tool, "", "")
				return err
			},
		},
		{
			name:                    "wrong params",
			expectExplicitTaskQuery: true,
			call: func(ctx context.Context) error {
				_, err := react._invokeToolCall_ReviewWrongParamForTask(
					ctx,
					task,
					tool,
					aitool.InvokeParams{"input": "old"},
					"review suggestion",
				)
				return err
			},
		},
		{
			name:                    "interval review",
			expectExplicitTaskQuery: true,
			call: func(ctx context.Context) error {
				_, err := react._invokeToolCall_IntervalReviewWithContextForTask(
					ctx,
					task,
					tool,
					aitool.InvokeParams{"input": "value"},
					nil,
					nil,
					time.Now(),
					1,
					"continue while progress is healthy",
				)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				done <- tt.call(ctx)
			}()

			select {
			case req := <-requests:
				require.NotNil(t, req.GetContext(), "sub-AI request must carry the call context")
				if tt.expectExplicitTaskQuery {
					require.Contains(t, req.GetPrompt(), "the explicit owning task query")
					require.NotContains(t, req.GetPrompt(), "query from an unrelated concurrent task")
				}
			// Prompt construction may walk the configured workspace and is
			// intentionally outside the assertion under test; -race makes that
			// setup materially slower on large worktrees.
			case <-time.After(10 * time.Second):
				cancel()
				t.Fatal("review sub-AI did not start")
			}

			cancelledAt := time.Now()
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
				require.Less(t, time.Since(cancelledAt), 500*time.Millisecond)
			case <-time.After(time.Second):
				t.Fatal("review sub-AI did not stop after call context cancellation")
			}
		})
	}
}
