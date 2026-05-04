package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// 关键词: aicache.Split 单测, P0-A5, review 模板段稳定性回归
//
// 这些测试覆盖 P0-A4 阶段重构后的 3 大 review prompt 模板:
//
//	ai-review-plan.txt / ai-review-task.txt / ai-review-tool-call.txt
//
// 用于:
//  1. 保证模板被 aicache splitter 识别为多段 chunk (不再是 BACKGROUND 自定义 tag 单一散文)
//  2. 至少包含 high-static 与 dynamic 两段
//  3. high-static 段 hash 在跨调用 (不同 nonce) 下保持稳定

func chunksBySection(t *testing.T, prompt string) map[string][]*aicache.Chunk {
	t.Helper()
	require.NotEmpty(t, prompt, "split target prompt should not be empty")
	res := aicache.Split(prompt)
	require.NotNil(t, res)
	out := make(map[string][]*aicache.Chunk)
	for _, c := range res.Chunks {
		require.NotNil(t, c)
		out[c.Section] = append(out[c.Section], c)
	}
	return out
}

func TestSplit_PlanReviewPrompt_FourSections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := NewTestConfig(ctx)

	materials := aitool.InvokeParams{
		"plan_summary": "step 1 -> step 2",
		"plan_id":      "plan-1",
	}
	prompt1, err := generatePlanReviewPrompt(cfg, materials)
	require.NoError(t, err)
	prompt2, err := generatePlanReviewPrompt(cfg, materials)
	require.NoError(t, err)

	sec1 := chunksBySection(t, prompt1)
	sec2 := chunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic], "plan-review prompt must expose high-static chunk")
	require.NotEmpty(t, sec1[aicache.SectionDynamic], "plan-review prompt must expose dynamic chunk")
	require.Empty(t, sec1[aicache.SectionRaw], "plan-review prompt should not produce raw/noise chunk; rendered:\n%s", prompt1)

	require.Equal(t, sec1[aicache.SectionHighStatic][0].Hash, sec2[aicache.SectionHighStatic][0].Hash,
		"plan-review high-static hash must be byte-stable across calls")
}

func TestSplit_TaskReviewPrompt_FourSections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := NewTestConfig(ctx)

	materials := aitool.InvokeParams{
		"short_summary": "OK",
		"long_summary":  "task done",
		"progress":      "1-1 done",
		"pending_tasks": "1-2",
	}
	prompt1, err := generateTaskReviewPrompt(cfg, materials)
	require.NoError(t, err)
	prompt2, err := generateTaskReviewPrompt(cfg, materials)
	require.NoError(t, err)

	sec1 := chunksBySection(t, prompt1)
	sec2 := chunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic], "task-review prompt must expose high-static chunk")
	require.NotEmpty(t, sec1[aicache.SectionDynamic], "task-review prompt must expose dynamic chunk")
	require.Empty(t, sec1[aicache.SectionRaw], "task-review prompt should not produce raw/noise chunk; rendered:\n%s", prompt1)

	require.Equal(t, sec1[aicache.SectionHighStatic][0].Hash, sec2[aicache.SectionHighStatic][0].Hash,
		"task-review high-static hash must be byte-stable across calls")
}

func TestSplit_ToolCallReviewPrompt_FourSections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := NewTestConfig(ctx)

	prompt1, err := GenerateAIReviewPrompt(cfg, "ls /tmp", "bash", `{"command":"ls /tmp"}`)
	require.NoError(t, err)
	prompt2, err := GenerateAIReviewPrompt(cfg, "ls /tmp", "bash", `{"command":"ls /tmp"}`)
	require.NoError(t, err)

	sec1 := chunksBySection(t, prompt1)
	sec2 := chunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic], "tool-call review prompt must expose high-static chunk")
	require.NotEmpty(t, sec1[aicache.SectionDynamic], "tool-call review prompt must expose dynamic chunk")
	require.Empty(t, sec1[aicache.SectionRaw], "tool-call review prompt should not produce raw/noise chunk; rendered:\n%s", prompt1)

	require.Equal(t, sec1[aicache.SectionHighStatic][0].Hash, sec2[aicache.SectionHighStatic][0].Hash,
		"tool-call review high-static hash must be byte-stable across calls")
}
