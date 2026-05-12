package aid

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
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
	Timeline        string
	Persistent      string
}

func renderTaskSummaryFixture(t *testing.T, fixture taskSummaryFixture) string {
	t.Helper()
	prompt, err := assembleTaskSummaryPrompt(
		fixture.Schema,
		fixture.Persistent,
		fixture.Timeline,
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
		Timeline:        "interval-1 -> shell ls\ninterval-2 -> shell ps",
		Persistent:      "<persistent>vm-host=darwin</persistent>",
	}

	prompt1 := renderTaskSummaryFixture(t, stub)
	prompt2 := renderTaskSummaryFixture(t, stub)

	sec1 := taskSummaryChunksBySection(t, prompt1)
	sec2 := taskSummaryChunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic], "task-summary prompt must expose high-static chunk")
	require.NotEmpty(t, sec1[aicache.SectionDynamic], "task-summary prompt must expose dynamic chunk")
	require.Empty(t, sec1[aicache.SectionRaw], "task-summary prompt should not produce raw/noise chunk; rendered:\n%s", prompt1)

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
