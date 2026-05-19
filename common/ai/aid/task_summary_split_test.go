package aid

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

// 关键词: aicache.Split 单测, P0-A5, task-summary.txt 段稳定性回归
//
// task-summary prompt 通过 PromptPrefixBuilder 组装为多段 PROMPT_SECTION,
// 这里通过 assembleTaskSummaryPrompt 验证:
//  1. 切片输出多段 chunk, 包含 high-static 与 dynamic
//  2. 不出现 raw/noise chunk
//  3. high-static 段 hash 在跨调用下保持稳定 (任意 .ContextProvider 数据变化不影响)

type taskSummaryFixture struct {
	Schema          string
	CurrentTaskInfo string
	TimelineFrozen  string
	TimelineOpen    string
}

func taskSummaryToolConfig() *aicommon.Config {
	tool := aitool.NewWithoutCallback("grep", aitool.WithDescription("grep tool"))
	return &aicommon.Config{
		AiToolManager: buildinaitools.NewToolManagerByToolGetter(
			func() []*aitool.Tool { return []*aitool.Tool{tool} },
			buildinaitools.WithEnableAllTools(),
		),
		TopToolsCount: 100,
	}
}

func renderTaskSummaryFixture(t *testing.T, fixture taskSummaryFixture) string {
	t.Helper()
	prompt, err := assembleTaskSummaryPrompt(
		taskSummaryToolConfig(),
		fixture.Schema,
		fixture.TimelineFrozen,
		fixture.TimelineOpen,
		fixture.CurrentTaskInfo,
	)
	require.NoError(t, err)
	return prompt
}

func taskSummaryChunksBySection(t *testing.T, prompt string) map[string][]*aicache.Chunk {
	t.Helper()
	require.NotEmpty(t, prompt)
	res := aicache.Split(prompt)
	require.NotNil(t, res)
	out := make(map[string][]*aicache.Chunk)
	for _, c := range res.Chunks {
		require.NotNil(t, c)
		out[c.Section] = append(out[c.Section], c)
	}
	return out
}

func TestSplit_TaskSummaryPrompt_FourSections(t *testing.T) {
	stub := taskSummaryFixture{
		Schema:          `{"type":"object"}`,
		CurrentTaskInfo: "current task: scan /tmp",
		TimelineOpen:    "interval-2 -> shell ps",
	}

	prompt1 := renderTaskSummaryFixture(t, stub)
	prompt2 := renderTaskSummaryFixture(t, stub)

	sec1 := taskSummaryChunksBySection(t, prompt1)
	sec2 := taskSummaryChunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic], "task-summary prompt must expose high-static chunk")
	require.NotEmpty(t, sec1[aicache.SectionDynamic], "task-summary prompt must expose dynamic chunk")
	require.Empty(t, sec1[aicache.SectionRaw], "task-summary prompt should not produce raw/noise chunk; rendered:\n%s", prompt1)
	require.Contains(t, prompt1, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	require.Contains(t, prompt1, "# Tool Inventory")
	require.Contains(t, prompt1, "`grep`: grep tool")
	require.NotContains(t, prompt1, "# 牢记")

	require.Equal(t, sec1[aicache.SectionHighStatic][0].Hash, sec2[aicache.SectionHighStatic][0].Hash,
		"task-summary high-static hash must be byte-stable across calls")

	// 同一 schema 下, 不同 dynamic 输入仍保持 high-static / semi-dynamic 稳定。
	stub.CurrentTaskInfo = "current task: scan /var"
	prompt3 := renderTaskSummaryFixture(t, stub)
	sec3 := taskSummaryChunksBySection(t, prompt3)
	require.Equal(t, sec1[aicache.SectionHighStatic][0].Hash, sec3[aicache.SectionHighStatic][0].Hash,
		"task-summary high-static hash must remain stable across different dynamic inputs")
	if len(sec1[aicache.SectionSemiDynamic1]) > 0 && len(sec3[aicache.SectionSemiDynamic1]) > 0 {
		require.Equal(t, sec1[aicache.SectionSemiDynamic1][0].Hash, sec3[aicache.SectionSemiDynamic1][0].Hash,
			"task-summary semi-dynamic-1 hash should remain stable when only dynamic input changes")
	}
}

func TestSplit_TaskSummaryPrompt_FrozenTimelineLandsInFrozenBlock(t *testing.T) {
	stub := taskSummaryFixture{
		Schema:          `{"type":"object"}`,
		CurrentTaskInfo: "current task: scan /tmp",
		TimelineFrozen:  "<|TIMELINE_r1t100|>\nfrozen task timeline\n<|TIMELINE_END_r1t100|>",
		TimelineOpen:    "<|TIMELINE_r2t200|>\nopen task timeline\n<|TIMELINE_END_r2t200|>",
	}

	prompt := renderTaskSummaryFixture(t, stub)
	require.Contains(t, prompt, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	require.Contains(t, prompt, "# Tool Inventory")
	require.Contains(t, prompt, "`grep`: grep tool")
	require.Contains(t, prompt, "frozen task timeline")
	require.Contains(t, prompt, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	require.Contains(t, prompt, "<|PROMPT_SECTION_timeline-open|>")

	frozenStart := strings.Index(prompt, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	frozenEnd := strings.Index(prompt, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	openStart := strings.Index(prompt, "<|PROMPT_SECTION_timeline-open|>")
	require.GreaterOrEqual(t, frozenStart, 0)
	require.Greater(t, frozenEnd, frozenStart)
	require.Greater(t, openStart, frozenEnd)
	require.NotContains(t, prompt[frozenStart:frozenEnd], "open task timeline")
	require.NotContains(t, prompt[frozenStart:frozenEnd], "<summary>")
	require.Contains(t, prompt[openStart:], "open task timeline")
	require.NotContains(t, prompt[openStart:], "<summary>")
}

func TestGenerateTaskSummaryPrompt_UsesConfigTimelineFrozenOpen(t *testing.T) {
	_, task, _, _ := newPlanExecPromptFixture(t)
	timeline := task.ContextProvider.GetTimelineInstance()
	require.NotNil(t, timeline)
	task.Config.Timeline = timeline
	task.Config.AiToolManager = taskSummaryToolConfig().AiToolManager
	task.Config.TopToolsCount = 100
	timeline.SetTimelineBucketByteSize(80)

	timeline.PushText(101, "first current task timeline block "+strings.Repeat("A", 120))
	timeline.PushText(102, "second current task timeline block "+strings.Repeat("B", 120))

	prompt, err := task.GenerateTaskSummaryPrompt()
	require.NoError(t, err)
	require.Contains(t, prompt, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	require.Contains(t, prompt, "`grep`: grep tool")
	require.Contains(t, prompt, "first current task timeline block")
	require.Contains(t, prompt, "<|PROMPT_SECTION_timeline-open|>")
	require.Contains(t, prompt, "second current task timeline block")
	frozenEnd := strings.Index(prompt, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	openStart := strings.Index(prompt, "<|PROMPT_SECTION_timeline-open|>")
	require.Greater(t, frozenEnd, 0)
	require.Greater(t, openStart, frozenEnd, "frozen timeline must be outside and before timeline-open")
}
