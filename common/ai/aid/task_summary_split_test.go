package aid

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
)

// 关键词: aicache.Split 单测, P0-A5, task-summary.txt 段稳定性回归
//
// task-summary.txt 在 P0-A2 阶段被改造成 4 段 PROMPT_SECTION 包装,
// 这里通过直接渲染模板字符串验证:
//  1. 切片输出多段 chunk, 包含 high-static 与 dynamic
//  2. 不出现 raw/noise chunk
//  3. high-static 段 hash 在跨调用下保持稳定 (任意 .ContextProvider 数据变化不影响)

type taskSummaryStubSchema struct {
	TaskSummarySchema string
}

type taskSummaryStubProvider struct {
	CurrentTaskInfo     string
	CurrentTaskTimeline string
	PersistentMemory    string
	Schema              taskSummaryStubSchema
}

func renderTaskSummaryFixture(t *testing.T, provider taskSummaryStubProvider) string {
	t.Helper()
	tmpl, err := template.New("task-summary-test").Parse(__prompt_TaskSummary)
	require.NoError(t, err)
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]any{
		"ContextProvider": provider,
	})
	require.NoError(t, err)
	return buf.String()
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
	stub := taskSummaryStubProvider{
		CurrentTaskInfo:     "current task: scan /tmp",
		CurrentTaskTimeline: "interval-1 -> shell ls\ninterval-2 -> shell ps",
		PersistentMemory:    "<persistent>vm-host=darwin</persistent>",
		Schema: taskSummaryStubSchema{
			TaskSummarySchema: `{"type":"object"}`,
		},
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
	if len(sec1[aicache.SectionSemiDynamic]) > 0 && len(sec3[aicache.SectionSemiDynamic]) > 0 {
		require.Equal(t, sec1[aicache.SectionSemiDynamic][0].Hash, sec3[aicache.SectionSemiDynamic][0].Hash,
			"task-summary semi-dynamic hash should remain stable when only dynamic input changes")
	}
}
