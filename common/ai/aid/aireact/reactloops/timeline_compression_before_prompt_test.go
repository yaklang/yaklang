package reactloops

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

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

func (i *compressionBeforePromptInvoker) AssembleLoopPrompt(_ []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	view := aicommon.RenderTimelineFrozenOpen(i.timeline)
	prompt := view.Frozen + view.PromotedUserInputHistory + view.Open
	if input != nil {
		prompt += input.TaskInstruction + input.FrozenUserContext + input.TodoSnapshot + input.Schema + input.OutputExample + input.ReactiveData
	}
	return &aicommon.LoopPromptAssemblyResult{Prompt: prompt}, nil
}

func TestTimelineCompressionBeforeLoopPrompt(t *testing.T) {
	ctx := context.Background()
	cfg := &compressionBeforePromptConfig{}
	cfg.Config = aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(1200),
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			cfg.calls++
			cfg.prompt = req.GetPrompt()
			require.Equal(t, aicommon.CallerLabelTimelineCompress, req.GetCallerLabel())
			if cfg.beforeCompress != nil {
				cfg.beforeCompress()
			}
			response := aicommon.NewUnboundAIResponse()
			response.EmitOutputStream(strings.NewReader(`{"@action":"compress_timeline","summary":"COMPACTED_BEFORE_ASSEMBLY","retain_timeline":"","retire_skill_names":[],"retire_evidence_ids":[],"memory_entities":[]}`))
			response.Close()
			return response, nil
		}))
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
	cfg.Timeline.PushText(1, strings.Repeat("ORIGINAL_HISTORY ", 2000))
	cfg.Timeline.PushUserInteraction(aicommon.UserInteractionStage_FreeInput, 100, "", "CURRENT_QUERY")
	loop.persistentInstructionProvider = func(*ReActLoop, string) (string, error) { return "CURRENT_INSTRUCTION", nil }
	build := loop.captureLoopPromptBuildInput("compression-before-prompt", "CURRENT_QUERY", "FROZEN_USER_CONTEXT", nil, "", &LoopActionHandlerOperator{})
	prompt, err := loop.generateLoopPromptFromInput(build)
	require.NoError(t, err)
	require.Contains(t, prompt, "ORIGINAL_HISTORY")
	require.Greater(t, int64(aicommon.MeasureTokens(prompt)), loop.wholePromptTokenLimit())
	require.NoError(t, loop.compressMainPrompt(build, loop.wholePromptTokenLimit()))
	prompt, err = loop.generateLoopPromptFromInput(build)
	require.NoError(t, err)
	require.Contains(t, prompt, "COMPACTED_BEFORE_ASSEMBLY")
	require.NotContains(t, prompt, "ORIGINAL_HISTORY")
	loop.WaitForInflightObservation()
	require.Equal(t, 1, cfg.calls)
	require.Contains(t, cfg.prompt, "ORIGINAL_HISTORY")
	require.Contains(t, cfg.prompt, "CURRENT_QUERY")
	require.Contains(t, cfg.prompt, "FROZEN_USER_CONTEXT")
	require.Contains(t, cfg.prompt, "# 压缩 Timeline")
	require.Contains(t, cfg.prompt, "retain_timeline")
	require.Contains(t, cfg.prompt, "retire_skill_names")
	require.Contains(t, cfg.prompt, "retire_evidence_ids")
	require.Contains(t, cfg.prompt, "ID=1")
	require.Contains(t, cfg.prompt, "当前整份主循环 prompt：")
	require.Zero(t, loop.compressionTurn, "the large prompt must not be held after compression")
	require.NotContains(t, cfg.prompt, "ITEM_ID=")
	require.NotContains(t, cfg.prompt, "ratain_timeline_item_range")
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
		cfg.Timeline.PushText(int64(i+1), strings.Repeat("NEW_HISTORY ", 2000))
		rebuild := loop.captureLoopPromptBuildInput("compression-before-prompt", "CURRENT_QUERY", "", nil, "", &LoopActionHandlerOperator{})
		require.NoError(t, loop.compressMainPrompt(rebuild, loop.wholePromptTokenLimit()))
		statuses = capture.snapshot()
		label := statuses[len(statuses)-2]
		require.Equal(t, "context.optimizing", label.Code)
		require.NotEmpty(t, label.ValueI18n.En)
		require.False(t, labels[label.Value], "consecutive optimizations should vary their wording")
		labels[label.Value] = true
	}
}

func TestTimelineCompressionExamplesSupportScatteredRanges(t *testing.T) {
	examples := regexp.MustCompile(`(?m)^\{.*\}$`).FindAllString(compressTimelineDynamicText, -1)
	require.Len(t, examples, 5)
	var selections []string
	for _, example := range examples {
		var params aitool.InvokeParams
		require.NoError(t, json.Unmarshal([]byte(example), &params))
		selection, ok := params["retain_timeline"].(string)
		require.True(t, ok)
		selections = append(selections, selection)
	}
	require.Contains(t, selections, "3-8,20-26,41-45")
	require.Contains(t, selections, "3-8,12,20-26,37,41-45")
	require.Contains(t, selections, "")
}

func TestTimelineCompressionTransactionCorrectsInvalidRanges(t *testing.T) {
	ctx := context.Background()
	const base = " \nSTATIC_PREFIX\nCURRENT_TASK_AND_TIMELINE\n  "
	var requests []string
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			requests = append(requests, req.GetPrompt())
			require.True(t, strings.HasPrefix(req.GetPrompt(), base), "compression must preserve the entire main-loop prompt byte for byte")
			selection := "3-8,12,20-26,37,41-45"
			if len(requests) == 1 {
				selection = "3-999"
			} else {
				require.Contains(t, req.GetPrompt(), "retain_timeline 范围")
			}
			payload, err := json.Marshal(map[string]any{
				"@action": "compress_timeline", "summary": "COMPACTED_SCATTERED_HISTORY", "retain_timeline": selection,
				"retire_skill_names": []string{}, "retire_evidence_ids": []string{},
			})
			require.NoError(t, err)
			response := aicommon.NewUnboundAIResponse()
			response.EmitOutputStream(strings.NewReader(string(payload)))
			response.Close()
			return response, nil
		}))
	for _, id := range []int64{3, 8, 10, 12, 20, 26, 37, 41, 45} {
		cfg.Timeline.PushText(id, "history-%d", id)
	}
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.invoker = mock.NewMockInvoker(ctx)
	decision, err := cfg.Timeline.BeginCompressionDecision(aicommon.TimelineCompressionOptions{Context: ctx})
	require.NoError(t, err)
	t.Cleanup(decision.Abort)
	loop.compressionTurn = timelineCompressionTurnContext{Prompt: base}
	require.NoError(t, loop.completeCompressionTurn(decision))
	require.Len(t, requests, 2, "the AI transaction should correct the selection without an extra compression loop")
	view := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.Contains(t, view.Frozen, "COMPACTED_SCATTERED_HISTORY")
	require.Contains(t, view.Frozen, "history-12")
	require.NotContains(t, view.Frozen, "history-10")
}
