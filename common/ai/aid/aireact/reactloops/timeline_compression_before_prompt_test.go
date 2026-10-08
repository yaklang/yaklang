package reactloops

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type compressionBeforePromptConfig struct {
	*aicommon.Config
	calls          int
	prompt         string
	beforeCompress func()
}

type compressionBeforePromptInvoker struct {
	*mock.MockInvoker
	timeline *aicommon.Timeline
}

func (i *compressionBeforePromptInvoker) AssembleLoopPrompt(_ []*aitool.Tool, _ *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	view := aicommon.RenderTimelineFrozenOpen(i.timeline)
	return &aicommon.LoopPromptAssemblyResult{Prompt: view.Frozen + view.Open}, nil
}

func (c *compressionBeforePromptConfig) ScheduleAuxiliaryTask(ctx context.Context, name string,
	builder func() string, result func(*aicommon.Action), opts ...aicommon.AuxiliaryTaskOption) {
	c.calls++
	c.prompt = builder()
	if c.beforeCompress != nil {
		c.beforeCompress()
	}
	action, err := aicommon.ExtractValidActionFromStream(ctx, strings.NewReader(`{"@action":"timeline-summary","summary":"COMPACTED_BEFORE_ASSEMBLY","ratain_timeline_item_range":"","memory_entities":[]}`), "timeline-summary")
	if err == nil {
		result(action)
	}
}

func TestTimelineCompressionBeforeLoopPrompt(t *testing.T) {
	ctx := context.Background()
	cfg := &compressionBeforePromptConfig{Config: aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(1))}
	cfg.Timeline.SoftBindConfig(cfg, nil)
	loop := makeSchemaStabilityTestLoop(cfg)
	capture := captureActivityStatus(loop)
	cfg.beforeCompress = func() {
		statuses := capture.snapshot()
		require.NotEmpty(t, statuses)
		require.Equal(t, "context.optimizing", statuses[len(statuses)-1].Code,
			"optimization status must arrive before the compression model returns")
	}
	loop.invoker = &compressionBeforePromptInvoker{MockInvoker: mock.NewMockInvoker(ctx), timeline: cfg.Timeline}
	cfg.Timeline.PushText(1, "ORIGINAL_HISTORY")
	loop.persistentInstructionProvider = func(*ReActLoop, string) (string, error) { return "CURRENT_INSTRUCTION", nil }
	prompt, err := loop.generateLoopPrompt("compression-before-prompt", "CURRENT_QUERY", "FROZEN_USER_CONTEXT", nil, "", &LoopActionHandlerOperator{})
	require.NoError(t, err)
	require.Contains(t, prompt, "COMPACTED_BEFORE_ASSEMBLY")
	require.NotContains(t, prompt, "ORIGINAL_HISTORY")
	loop.WaitForInflightObservation()
	require.Equal(t, 1, cfg.calls)
	require.Contains(t, cfg.prompt, "ORIGINAL_HISTORY")
	require.Contains(t, cfg.prompt, "CURRENT_QUERY")
	require.NotContains(t, cfg.prompt, "CURRENT_INSTRUCTION")
	require.NotContains(t, cfg.prompt, "TASK_INSTRUCTION")
	require.Contains(t, cfg.prompt, "FROZEN_USER_CONTEXT")
	view := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.Contains(t, view.Frozen, "COMPACTED_BEFORE_ASSEMBLY")
	require.Empty(t, view.Open)
	_, err = loop.generateLoopPrompt("compression-before-prompt", "CURRENT_QUERY", "FROZEN_USER_CONTEXT", nil, "", &LoopActionHandlerOperator{})
	require.NoError(t, err)
	loop.WaitForInflightObservation()
	require.Equal(t, 1, cfg.calls, "unchanged history must not be compressed twice")
	statuses := capture.snapshot()
	require.Len(t, statuses, 2, "a skipped compression must not update the status")
	require.Equal(t, "response.preparing", statuses[1].Code, "completed optimization must release its status")
	labels := map[string]bool{statuses[0].Value: true}
	for i := 1; i < len(contextOptimizationStatusNames); i++ {
		cfg.Timeline.PushText(int64(i+1), "NEW_HISTORY")
		require.NoError(t, loop.compressTimelineBeforePrompt("CURRENT_QUERY", "", ""))
		statuses = capture.snapshot()
		label := statuses[len(statuses)-2]
		require.Equal(t, "context.optimizing", label.Code)
		require.NotEmpty(t, label.ValueI18n.En)
		require.False(t, labels[label.Value], "consecutive optimizations should vary their wording")
		labels[label.Value] = true
	}
}
