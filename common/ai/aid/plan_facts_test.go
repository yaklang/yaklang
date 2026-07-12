package aid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestExtractPlanContextFromText_UsesAITagParser(t *testing.T) {
	input := `prefix
<|FACTS_nonce1|>
## Facts
- a
<|FACTS_END_nonce1|>

middle

<|EVIDENCE_nonce2|>
## Evidence
- b
<|EVIDENCE_END_nonce2|>
suffix`

	require.Equal(t, "## Facts\n- a", extractPlanFactsFromText(input))
	require.Equal(t, "## Evidence\n- b", extractPlanEvidenceFromText(input))
}

func TestStripPlanContextBlocks_RemovesAITagBlocksOnly(t *testing.T) {
	input := `before

<|FACTS_nonce1|>
## Facts
- a
<|FACTS_END_nonce1|>

between

<|PLAN_EVIDENCE_nonce2|>
## Evidence
- b
<|PLAN_EVIDENCE_END_nonce2|>

after`

	require.Equal(t, "before\n\nbetween\n\nafter", stripPlanContextBlocks(input))
}

func TestBuildTaskPlanVerificationCarryoverMarkdown_IncludesVerificationAndDeliveryFiles(t *testing.T) {
	cod := &Coordinator{Config: &aicommon.Config{Ctx: context.Background()}}
	task := cod.generateAITaskWithName("系统配置检查", "确认系统配置")
	task.Index = "1-2"
	markdown := buildTaskPlanVerificationCarryoverMarkdown(
		task,
		"目标达成：已确认目标主机操作系统类型为 darwin",
	)
	require.Contains(t, markdown, "系统配置检查 核实结果")
	require.Contains(t, markdown, "### 判定")
	require.NotContains(t, markdown, "交付文件")
}

func TestAppendTaskPlanEvidence_StoresVerificationCarryoverForSharedContext(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: mem,
		userInput:       "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	cur := cod.generateAITaskWithName("Current", "current goal")
	cur.Index = "1-1"
	cur.ParentTask = root
	root.Subtasks = []*AiTask{cur}

	mem.CurrentTask = cur
	mem.RootTask = root

	carryover := mergePlanContextDocuments(
		buildTaskPlanVerificationCarryoverMarkdown(cur, "目标达成：已确认操作系统为 darwin"),
		buildTaskPlanSummaryCarryoverMarkdown(cur, "### 关键结果\n- 已完成系统类型确认。"),
	)
	merged, changed := appendTaskPlanEvidence(cur, carryover)
	require.True(t, changed)
	require.Contains(t, merged, "核实结果")
	require.Contains(t, merged, "任务总结")
	require.Contains(t, mem.SharedEvidenceContext(), "已完成系统类型确认")
}