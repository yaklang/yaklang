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
	calls  int
	prompt string
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
	action, err := aicommon.ExtractValidActionFromStream(ctx, strings.NewReader(`{"@action":"timeline-summary","summary":"COMPACTED_BEFORE_ASSEMBLY"}`), "timeline-summary")
	if err == nil {
		result(action)
	}
}

func TestTimelineCompressionBeforeLoopPrompt(t *testing.T) {
	ctx := context.Background()
	cfg := &compressionBeforePromptConfig{Config: aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(1))}
	cfg.Timeline.SoftBindConfig(cfg, nil)
	loop := makeSchemaStabilityTestLoop(cfg)
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
	require.Contains(t, cfg.prompt, "CURRENT_INSTRUCTION")
	require.Contains(t, cfg.prompt, "FROZEN_USER_CONTEXT")
	view := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.Contains(t, view.Frozen, "COMPACTED_BEFORE_ASSEMBLY")
	require.Empty(t, view.Open)
	_, err = loop.generateLoopPrompt("compression-before-prompt", "CURRENT_QUERY", "FROZEN_USER_CONTEXT", nil, "", &LoopActionHandlerOperator{})
	require.NoError(t, err)
	loop.WaitForInflightObservation()
	require.Equal(t, 1, cfg.calls, "unchanged history must not be compressed twice")
}
