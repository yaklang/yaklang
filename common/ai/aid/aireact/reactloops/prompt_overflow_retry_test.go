package reactloops

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func newPromptOverflowRetryTestLoop(t *testing.T, opts ...aicommon.ConfigOption) (*ReActLoop, *aicommon.Config) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	baseOpts := []aicommon.ConfigOption{
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, errors.New("prompt build must not invoke AI")
		}),
	}
	cfg := aicommon.NewConfig(ctx, append(baseOpts, opts...)...)
	cfg.Timeline = aicommon.NewTimeline(nil, nil)
	cfg.Timeline.SetTimelineContentLimit(cfg.GetTimelineContentSizeLimit())
	cfg.AiServerName, cfg.AiModelName = "openai", "gpt-4o"
	invoker := mock.NewMockInvoker(ctx)
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(cfg.DefaultTask)
	t.Cleanup(loop.Release)
	return loop, cfg
}

func promptOverflowErrorFor(provider, model string, maxContextTokens int64) error {
	return &aispec.PromptOverflowError{
		PromptOverflowEvidence: aispec.PromptOverflowEvidence{
			Provider:         provider,
			Model:            model,
			MaxContextTokens: maxContextTokens,
			Confidence:       aispec.PromptOverflowHigh,
		},
		Cause: errors.New("provider rejected the prompt"),
	}
}

// A confirmed overflow lowers the timeline limit for the model that rejected
// the prompt; a second identical overflow must not keep retrying.
func TestReduceTimelineLimitForPromptOverflow(t *testing.T) {
	loop, cfg := newPromptOverflowRetryTestLoop(t)
	base := cfg.GetTimelineContentSizeLimit()
	require.EqualValues(t, base, cfg.Timeline.GetTimelineContentLimit())

	overflow := promptOverflowErrorFor("openai", "gpt-4o", 128000)
	require.True(t, loop.reduceTimelineLimitForPromptOverflow(overflow, strings.Repeat("prompt ", 1000)))
	require.EqualValues(t, 102400, cfg.Timeline.GetTimelineContentLimit())

	require.False(t, loop.reduceTimelineLimitForPromptOverflow(overflow, strings.Repeat("prompt ", 1000)),
		"an identical overflow must stop the retry instead of looping")
	require.EqualValues(t, 102400, cfg.Timeline.GetTimelineContentLimit())
}

func TestSyncPromptOverflowTimelineLimitKeepsTheLowerBound(t *testing.T) {
	loop, cfg := newPromptOverflowRetryTestLoop(t)
	require.True(t, loop.reduceTimelineLimitForPromptOverflow(
		promptOverflowErrorFor("openai", "gpt-4o", 64000), "prompt"))
	require.EqualValues(t, 51200, cfg.Timeline.GetTimelineContentLimit())

	// The session keeps the reduced threshold on later prompt builds.
	loop.syncPromptOverflowTimelineLimit()
	require.EqualValues(t, 51200, cfg.Timeline.GetTimelineContentLimit())

	// Unknown evidence and non-config runtimes leave the limit untouched.
	require.False(t, loop.reduceTimelineLimitForPromptOverflow(errors.New("not an overflow"), "prompt"))
	require.EqualValues(t, 51200, cfg.Timeline.GetTimelineContentLimit())
}

func TestPromptOverflowRetryMaxReadsConfig(t *testing.T) {
	loop, _ := newPromptOverflowRetryTestLoop(t)
	require.Equal(t, 2, loop.promptOverflowRetryMax(), "default budget")

	disabled := newPromptOverflowConfig(t, aicommon.WithPromptOverflowRetryMax(0))
	require.Zero(t, disabled.GetPromptOverflowRetryMax())
}

func newPromptOverflowConfig(t *testing.T, opts ...aicommon.ConfigOption) *aicommon.Config {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return aicommon.NewConfig(ctx, append([]aicommon.ConfigOption{
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
	}, opts...)...)
}

// An overflow retry rebuilds the prompt without consuming the iteration's
// one-shot inputs twice: the todo checkpoint belongs to the whole iteration.
func TestPromptRebuildReusesTheTodoCheckpoint(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	setCurrentTodo(t, cfg, task, "todo-1")
	for iteration := 0; iteration < currentTodoCheckpointThreshold; iteration++ {
		loop.recordCurrentTodoIteration(task)
	}

	input := loop.captureLoopPromptBuildInput("retry-nonce", "user input", "", nil, "", &LoopActionHandlerOperator{})
	require.Equal(t, currentTodoCheckpointPrompt, input.todoCheckpoint)
	require.Empty(t, loop.consumeTodoCheckpoint(), "capture must consume the checkpoint exactly once")

	first, err := loop.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	second, err := loop.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	require.Contains(t, first.Prompt, "主要矛盾", "the first build carries the checkpoint")
	require.Contains(t, second.Prompt, "主要矛盾", "the rebuild reuses the same checkpoint")
}
