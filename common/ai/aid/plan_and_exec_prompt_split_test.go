package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func aidPromptChunksBySection(t *testing.T, prompt string) map[string][]*aicache.Chunk {
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

func firstChunk(t *testing.T, sections map[string][]*aicache.Chunk, name string) *aicache.Chunk {
	t.Helper()
	require.NotEmpty(t, sections[name], "expected section %s", name)
	return sections[name][0]
}

func newPlanExecPromptFixture(t *testing.T) (*Coordinator, *AiTask, *PlanResponse, *planRequest) {
	t.Helper()

	mem := GetDefaultContextProvider()
	mem.StoreQuery("请规划并执行这个任务")
	mem.StoreToolsKeywords(func() []string { return []string{"grep", "read_file"} })
	mem.SetPersistentData(planEvidencePersistentKey, "## 共享事实\n- /tmp/report.md 已存在")

	cod := &Coordinator{
		Config: &aicommon.Config{
			Ctx:             context.Background(),
			MaxTaskContinue: 10,
		},
		ContextProvider: mem,
		userInput:       "请规划并执行这个任务",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	root.SetUserInput("root input")
	task1 := cod.generateAITaskWithName("Analyze", "analyze current task")
	task1.Index = "1-1"
	task1.ParentTask = root
	task1.SetUserInput("collect evidence for current task")
	task2 := cod.generateAITaskWithName("Verify", "verify remaining path")
	task2.Index = "1-2"
	task2.ParentTask = root
	task2.SetUserInput("verify remaining path")
	root.Subtasks = []*AiTask{task1, task2}

	cod.rootTask = root
	mem.RootTask = root
	mem.CurrentTask = task1

	return cod, task1, &PlanResponse{RootTask: root}, &planRequest{cod: cod}
}

func renderDynamicPlanFixture(t *testing.T, task *AiTask, userInput string, frozen string, open string) string {
	t.Helper()
	schema := task.ContextProvider.Schema()
	materials := &aicommon.PromptMaterials{
		TaskInstruction: strings.TrimSpace(__prompt_dynamicPlanInstruction),
		Schema:            schema["RePlanJsonSchema"],
		TimelineFrozen:    frozen,
		TimelineOpen:      open,
	}
	prompt, err := aicommon.NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(
		materials,
		"aid-dynamic-plan-dynamic",
		__prompt_dynamicPlanDynamic,
		dynamicPlanDynamicData{
			CurrentTaskInfo:   task.ContextProvider.CurrentTaskInfoDynamic(),
			UserInput:         userInput,
			PlanHelp:          task.ContextProvider.PlanHelp(),
			StableInstruction: task.ContextProvider.CurrentTaskInfoStable(),
		},
		"dyn-plan-test",
	)
	require.NoError(t, err)
	return prompt
}

func TestSplit_DeepThinkPlanPrompt_CacheSections(t *testing.T) {
	_, task, _, _ := newPlanExecPromptFixture(t)

	prompt1, err := task.GenerateDeepThinkPlanPrompt("建议细化当前步骤")
	require.NoError(t, err)
	prompt2, err := task.GenerateDeepThinkPlanPrompt("建议补充检查步骤")
	require.NoError(t, err)

	sec1 := aidPromptChunksBySection(t, prompt1)
	sec2 := aidPromptChunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic1])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic2])
	require.NotEmpty(t, sec1[aicache.SectionDynamic])
	require.Empty(t, sec1[aicache.SectionRaw], "deepthink-plan should not produce raw chunks:\n%s", prompt1)

	require.Equal(t, firstChunk(t, sec1, aicache.SectionHighStatic).Hash, firstChunk(t, sec2, aicache.SectionHighStatic).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic1).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic1).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic2).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic2).Hash)

	require.NotContains(t, firstChunk(t, sec1, aicache.SectionHighStatic).Content, "<|PERSISTENT|>")
	require.NotContains(t, firstChunk(t, sec1, aicache.SectionHighStatic).Content, "<|OUTPUT_EXAMPLE|>")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionSemiDynamic2).Content, "<|PERSISTENT|>")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionSemiDynamic2).Content, "<|SCHEMA|>")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionDynamic).Content, "## 规划任务帮助信息")
}

func TestSplit_DynamicPlanPrompt_CacheSectionsAndFrozenOpen(t *testing.T) {
	_, task, _, _ := newPlanExecPromptFixture(t)

	frozen := "<|TIMELINE_r1t1|>\nfrozen timeline body\n<|TIMELINE_END_r1t1|>"
	open := "<|TIMELINE_b1t2|>\nopen timeline body\n<|TIMELINE_END_b1t2|>"

	prompt1 := renderDynamicPlanFixture(t, task, "第一次反馈", frozen, open)
	prompt2 := renderDynamicPlanFixture(t, task, "第二次反馈", frozen, open)

	sec1 := aidPromptChunksBySection(t, prompt1)
	sec2 := aidPromptChunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic1])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic2])
	require.NotEmpty(t, sec1[aicache.SectionTimelineOpen])
	require.NotEmpty(t, sec1[aicache.SectionDynamic])
	require.Empty(t, sec1[aicache.SectionRaw], "dynamic-plan should not produce raw chunks:\n%s", prompt1)

	require.Contains(t, prompt1, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	require.Contains(t, prompt1, "<|PROMPT_SECTION_timeline-open|>")
	require.NotContains(t, prompt1, "<|PROMPT_SECTION_timeline|>")

	require.Equal(t, firstChunk(t, sec1, aicache.SectionHighStatic).Hash, firstChunk(t, sec2, aicache.SectionHighStatic).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic1).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic1).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic2).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic2).Hash)

	require.NotContains(t, firstChunk(t, sec1, aicache.SectionHighStatic).Content, "<|PERSISTENT|>")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionSemiDynamic2).Content, "<|PERSISTENT|>")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionDynamic).Content, peTaskMarkerSharedEvidence)
}

func TestSplit_PlanReviewIncompletePrompt_CacheSections(t *testing.T) {
	_, _, rsp, pr := newPlanExecPromptFixture(t)

	prompt1, _, err := pr.buildPlanIncompletePrompt("incomplete", "补充提示A", rsp)
	require.NoError(t, err)
	prompt2, _, err := pr.buildPlanIncompletePrompt("incomplete", "补充提示B", rsp)
	require.NoError(t, err)

	sec1 := aidPromptChunksBySection(t, prompt1)
	sec2 := aidPromptChunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic1])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic2])
	require.NotEmpty(t, sec1[aicache.SectionDynamic])
	require.Empty(t, sec1[aicache.SectionRaw], "plan-incomplete should not produce raw chunks:\n%s", prompt1)

	require.Equal(t, firstChunk(t, sec1, aicache.SectionHighStatic).Hash, firstChunk(t, sec2, aicache.SectionHighStatic).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic1).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic1).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic2).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic2).Hash)
	require.NotContains(t, firstChunk(t, sec1, aicache.SectionHighStatic).Content, "<|PERSISTENT|>")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionDynamic).Content, "## 规划任务帮助信息")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionDynamic).Content, "## 最原始用户输入")
}

func TestSplit_PlanFreedomReviewPrompt_CacheSections(t *testing.T) {
	_, _, rsp, pr := newPlanExecPromptFixture(t)

	prompt1, _, err := pr.buildFreedomReviewPrompt("用户删掉了旧任务", rsp)
	require.NoError(t, err)
	prompt2, _, err := pr.buildFreedomReviewPrompt("用户新增了任务", rsp)
	require.NoError(t, err)

	sec1 := aidPromptChunksBySection(t, prompt1)
	sec2 := aidPromptChunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic1])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic2])
	require.NotEmpty(t, sec1[aicache.SectionDynamic])
	require.Empty(t, sec1[aicache.SectionRaw], "plan-freedom-review should not produce raw chunks:\n%s", prompt1)

	require.Equal(t, firstChunk(t, sec1, aicache.SectionHighStatic).Hash, firstChunk(t, sec2, aicache.SectionHighStatic).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic1).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic1).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic2).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic2).Hash)
	require.NotContains(t, firstChunk(t, sec1, aicache.SectionHighStatic).Content, "<|OUTPUT_EXAMPLE|>")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionDynamic).Content, "## 规划任务帮助信息")
	require.Contains(t, firstChunk(t, sec1, aicache.SectionDynamic).Content, "## 最原始用户输入")
}

func TestSplit_PlanCreateSubtaskPrompt_CacheSections(t *testing.T) {
	_, _, rsp, pr := newPlanExecPromptFixture(t)

	prompt1, _, err := pr.buildCreateSubtaskPrompt("请优先拆 1-1", []string{"1-1", "1-2"}, rsp)
	require.NoError(t, err)
	prompt2, _, err := pr.buildCreateSubtaskPrompt("请优先拆 1-2", []string{"1-1", "1-2"}, rsp)
	require.NoError(t, err)

	sec1 := aidPromptChunksBySection(t, prompt1)
	sec2 := aidPromptChunksBySection(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic1])
	require.NotEmpty(t, sec1[aicache.SectionSemiDynamic2])
	require.NotEmpty(t, sec1[aicache.SectionDynamic])
	require.Empty(t, sec1[aicache.SectionRaw], "plan-create-subtask should not produce raw chunks:\n%s", prompt1)

	require.Contains(t, prompt1, "## 重点拆分目标")
	require.Contains(t, prompt1, "1-1: Analyze")
	require.Contains(t, prompt1, "1-2: Verify")

	require.Equal(t, firstChunk(t, sec1, aicache.SectionHighStatic).Hash, firstChunk(t, sec2, aicache.SectionHighStatic).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic1).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic1).Hash)
	require.Equal(t, firstChunk(t, sec1, aicache.SectionSemiDynamic2).Hash, firstChunk(t, sec2, aicache.SectionSemiDynamic2).Hash)
	require.NotContains(t, firstChunk(t, sec1, aicache.SectionHighStatic).Content, "<|PERSISTENT|>")
	require.Contains(t, prompt1, "## 规划任务帮助信息")
	require.Contains(t, prompt1, "## 最原始用户输入")
}
