package aid

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"
)

func TestEvidencePlanContextUsesJournalWithoutDuplicateDynamicBlock(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	cfg.GetTimeline().SetTimelineBucketByteSize(-1)
	mem := GetDefaultContextProvider()
	cod := &Coordinator{Config: cfg, ContextProvider: mem, userInput: "test"}
	task := cod.generateAITaskWithName("Current", "current goal")
	task.Index = "1"
	mem.CurrentTask, mem.RootTask = task, task
	mem.SetPersistentData("unrelated_task_data", "UNRELATED_TASK_DATA")
	cfg.ApplySessionEvidenceOps(buildVerificationCarryoverEvidenceOps(task, "VERIFIED_JOURNAL_EVIDENCE"))
	cfg.ApplySessionEvidenceOps(buildSummaryEvidenceOps(task, "SUMMARY_JOURNAL_EVIDENCE"))
	require.Contains(t, cfg.GetSessionEvidenceRendered(), "VERIFIED_JOURNAL_EVIDENCE")
	require.Contains(t, cfg.GetSessionEvidenceRendered(), "SUMMARY_JOURNAL_EVIDENCE")
	require.NotContains(t, cfg.GetSessionEvidenceRendered(), "UNRELATED_TASK_DATA")
	dynamic := mem.CurrentTaskInfoDynamic()
	require.NotContains(t, dynamic, "UNRELATED_TASK_DATA")
	require.NotContains(t, dynamic, "VERIFIED_JOURNAL_EVIDENCE")
	require.NotContains(t, dynamic, "共享执行证据")
	require.Contains(t, mem.CurrentTaskInfoStable(), "先决条件检查")
	before := aicommon.BuildPromptFrozenOpenMaterials(cfg)
	require.Contains(t, before.TimelineOpen, "VERIFIED_JOURNAL_EVIDENCE")
	require.Empty(t, before.SessionEvidenceSemiDynamic)
	cfg.GetTimeline().FreezeAll()
	after := aicommon.BuildPromptFrozenOpenMaterials(cfg)
	require.Contains(t, after.SessionEvidenceSemiDynamic, "VERIFIED_JOURNAL_EVIDENCE")
	require.Empty(t, after.TimelineOpen)
}
