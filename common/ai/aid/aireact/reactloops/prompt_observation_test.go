package reactloops

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type promptObservationTestInvoker struct {
	*mock.MockInvoker
}

func (i *promptObservationTestInvoker) AssembleLoopPrompt(tools []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	_ = tools
	highStatic := NewPromptContainerSection("section.high_static", "Highly Static", PromptSectionRoleSystemPrompt)
	highStatic.Children = []*PromptSectionObservation{
		NewPromptSectionObservation("section.high_static.task_instruction", "Task Instruction", PromptSectionRoleSystemPrompt, false, "# Task Instruction\npersistent instruction"),
		NewPromptSectionObservation("section.high_static.output_example", "Output Example", PromptSectionRoleSystemPrompt, false, "# Output Example\nexample output"),
	}
	highStatic = FinalizePromptContainerSection(highStatic)

	semiDynamic := NewPromptContainerSection("section.semi_dynamic", "Semi Dynamic", PromptSectionRoleMixed)
	semiDynamic.Children = []*PromptSectionObservation{
		NewPromptSectionObservation("section.semi_dynamic.skills_context", "Skills Context", PromptSectionRoleRuntimeCtx, true, "# Skills Context\nloaded skill-a"),
		NewPromptSectionObservation("section.semi_dynamic.schema", "Schema", PromptSectionRoleSystemPrompt, false, "# Schema\n{\"type\":\"object\"}"),
	}
	semiDynamic = FinalizePromptContainerSection(semiDynamic)

	timeline := NewPromptContainerSection("section.timeline", "Timeline", PromptSectionRoleMixed)
	timeline.Children = []*PromptSectionObservation{
		NewPromptSectionObservation("section.timeline.timeline", "Timeline Memory", PromptSectionRoleRuntimeCtx, true, "# Timeline Memory\nstep1\nstep2"),
		NewPromptSectionObservation("section.timeline.current_time", "Current Time", PromptSectionRoleRuntimeCtx, false, "# Current Time\n2026-04-01 12:00:00"),
	}
	timeline = FinalizePromptContainerSection(timeline)

	dynamic := NewPromptContainerSection("section.dynamic", "Pure Dynamic", PromptSectionRoleMixed)
	dynamic.Children = []*PromptSectionObservation{
		NewPromptSectionObservation("section.dynamic.user_query", "User Query", PromptSectionRoleUserInput, false, "<|USER_QUERY_"+input.Nonce+"|>\nraw user input\n<|USER_QUERY_END_"+input.Nonce+"|>"),
		NewPromptSectionObservation("section.dynamic.reactive_data", "Reactive Data", PromptSectionRoleRuntimeCtx, true, "<|REFLECTION_"+input.Nonce+"|>\nreactive context\n<|REFLECTION_END_"+input.Nonce+"|>"),
		NewPromptSectionObservation("section.dynamic.injected_memory", "Injected Memory", PromptSectionRoleRuntimeCtx, true, "<|INJECTED_MEMORY_"+input.Nonce+"|>\nmemory content\n<|INJECTED_MEMORY_END_"+input.Nonce+"|>"),
	}
	dynamic = FinalizePromptContainerSection(dynamic)

	sections := []*PromptSectionObservation{highStatic, semiDynamic, timeline, dynamic}
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_high-static|>\n" + strings.TrimSpace(highStatic.Children[0].Content+"\n\n"+highStatic.Children[1].Content) + "\n<|PROMPT_SECTION_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\n" + strings.TrimSpace(semiDynamic.Children[0].Content+"\n\n"+semiDynamic.Children[1].Content) + "\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_timeline|>\n" + strings.TrimSpace(timeline.Children[0].Content+"\n\n"+timeline.Children[1].Content) + "\n<|PROMPT_SECTION_END_timeline|>",
		"<|PROMPT_SECTION_dynamic_" + input.Nonce + "|>\n" + strings.TrimSpace(dynamic.Children[0].Content+"\n\n"+dynamic.Children[1].Content+"\n\n"+dynamic.Children[2].Content) + "\n<|PROMPT_SECTION_dynamic_END_" + input.Nonce + "|>",
	}, "\n\n")
	return &aicommon.LoopPromptAssemblyResult{
		Prompt:   prompt,
		Sections: sections,
	}, nil
}

func TestGenerateLoopPrompt_RecordsObservation(t *testing.T) {
	invoker := &promptObservationTestInvoker{MockInvoker: mock.NewMockInvoker(context.Background())}
	config := invoker.GetConfig()
	loop := &ReActLoop{
		invoker:           invoker,
		loopName:          "prompt-observation-loop",
		config:            config,
		emitter:           config.GetEmitter(),
		actions:           omap.NewEmptyOrderedMap[string, *LoopAction](),
		loopActions:       omap.NewEmptyOrderedMap[string, LoopActionFactory](),
		streamFields:      omap.NewEmptyOrderedMap[string, *LoopStreamField](),
		aiTagFields:       omap.NewEmptyOrderedMap[string, *LoopAITagField](),
		vars:              omap.NewEmptyOrderedMap[string, any](),
		currentMemories:   omap.NewEmptyOrderedMap[string, *aicommon.MemoryEntity](),
		extraCapabilities: NewExtraCapabilitiesManager(),
	}
	loop.actions.Set(loopAction_DirectlyAnswer.ActionType, loopAction_DirectlyAnswer)
	loop.actions.Set(loopAction_Finish.ActionType, loopAction_Finish)
	WithPersistentInstruction("persistent instruction")(loop)
	WithReflectionOutputExample("example output")(loop)
	WithReactiveDataBuilder(func(loop *ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
		return "reactive context", nil
	})(loop)

	task := &mockSimpleTask{id: "test-task", index: "test-index"}
	operator := NewActionHandlerOperator(task)

	prompt, err := loop.generateLoopPrompt("nonce1", "raw user input", "memory content", operator)
	require.NoError(t, err)
	require.NotEmpty(t, prompt)

	observation := loop.GetLastPromptObservation()
	require.NotNil(t, observation)
	require.Equal(t, "prompt-observation-loop", observation.LoopName)
	require.Equal(t, "nonce1", observation.Nonce)
	require.Equal(t, len(prompt), observation.PromptBytes)
	require.Equal(t, ytoken.CalcTokenCount(prompt), observation.PromptTokens)
	require.Len(t, observation.Sections, 4)
	require.Greater(t, observation.SectionCount, len(observation.Sections))

	require.Equal(t, "section.high_static", observation.Sections[0].Key)
	require.Equal(t, PromptSectionRoleSystemPrompt, observation.Sections[0].Role)
	require.NotEmpty(t, observation.Sections[0].Children)
	require.Equal(t, "section.high_static.task_instruction", observation.Sections[0].Children[0].Key)
	require.Greater(t, observation.Sections[0].ContentBytes(), 0)
	require.Greater(t, observation.Sections[0].LineCount(), 0)
	require.Empty(t, observation.Sections[0].Content)
	require.Equal(t, "section.semi_dynamic", observation.Sections[1].Key)
	require.Equal(t, "section.timeline", observation.Sections[2].Key)
	require.Equal(t, "section.dynamic", observation.Sections[3].Key)
	require.Equal(t, "section.dynamic.user_query", observation.Sections[3].Children[0].Key)
	require.True(t, observation.Sections[3].Children[1].Compressible)
	require.True(t, observation.Sections[3].Children[2].Compressible)
	require.NotZero(t, observation.Stats.UserInputBytes)
	require.NotZero(t, observation.Stats.RuntimeCtxBytes)
	require.NotZero(t, observation.Stats.SystemPromptBytes)

	report := observation.RenderCLIReport(80)
	t.Logf("prompt observation cli report:\n%s", report)
	require.Contains(t, report, "Prompt Bytes:")
	require.Contains(t, report, "Section Tree")
	require.Contains(t, report, "Task Instruction")
	require.Contains(t, report, "key: section.dynamic.user_query")
	require.Contains(t, report, "meta: role=user_input, mode=fixed, included=yes")
	require.Contains(t, report, "raw user input")
	require.NotContains(t, report, "Unified Capability Loading")

	status := loop.GetLastPromptObservationStatus()
	require.NotNil(t, status)
	require.Equal(t, observation.LoopName, status.LoopName)
	require.Equal(t, observation.Nonce, status.Nonce)
	require.Equal(t, observation.PromptBytes, status.PromptBytes)
	require.Equal(t, observation.PromptTokens, status.PromptTokens)
	require.NotEmpty(t, status.Sections)
	require.Equal(t, "section.high_static", status.Sections[0].Key)
	require.NotEmpty(t, status.Sections[0].Children)
	require.Greater(t, status.Sections[0].Bytes, 0)
	require.Greater(t, status.Sections[0].Lines, 0)
	require.Empty(t, status.Sections[0].Summary)
	require.Equal(t, "section.high_static.task_instruction", status.Sections[0].Children[0].Key)
	require.Equal(t, "section.dynamic", status.Sections[3].Key)
	require.Equal(t, "section.dynamic.user_query", status.Sections[3].Children[0].Key)
	require.Contains(t, status.Sections[3].Children[0].Summary, "raw user input")

	// 新增字段验证: bytes_percent / estimated_tokens / content_hash / summary_truncated
	// 关键词: prompt_profile 新字段, BytesPercent, EstimatedTokens, ContentHash, SummaryTruncated
	for _, top := range status.Sections {
		require.GreaterOrEqual(t, top.BytesPercent, 0.0)
		require.LessOrEqual(t, top.BytesPercent, 100.0)
		// 容器段 ContentHash 取自 (空) Content -> 仍允许为空, 子段必非空 hash
		for _, child := range top.Children {
			if child.Bytes > 0 {
				require.Len(t, child.ContentHash, 8, "child content_hash should be 8 hex chars")
				require.Greater(t, child.EstimatedTokens, 0)
			}
		}
	}
	// timeline 段子段 "Timeline Memory" 内容含两行 step1 / step2, summary 必须保留换行
	timelineMemory := status.Sections[2].Children[0]
	require.Equal(t, "section.timeline.timeline", timelineMemory.Key)
	require.Contains(t, timelineMemory.Summary, "step1")
	require.Contains(t, timelineMemory.Summary, "step2")
	require.True(t, strings.Contains(timelineMemory.Summary, "\n"),
		"summary must keep newlines (no longer flattened to single line)")

	loop.Set("prompt_observation_log", true)
}

// TestPreviewSectionContent_KeepNewlinesAndTruncate 单测 previewSectionContent
// 1. 内容小于上限 -> 原样返回, truncated=false
// 2. 内容超过上限 -> 头部前缀截断 + 末尾追加 "... (truncated, total N bytes)" + truncated=true
// 3. 换行不被压平 (老的 renderPromptSectionPreview 会把 \n 替成空格, 这里禁止该行为)
// 关键词: previewSectionContent test, 保留换行, 头部截断, truncated 提示
func TestPreviewSectionContent_KeepNewlinesAndTruncate(t *testing.T) {
	short := "line1\nline2\nline3"
	preview, truncated := previewSectionContent(short, 1024)
	require.False(t, truncated)
	require.Equal(t, short, preview)
	require.Contains(t, preview, "\n")

	long := strings.Repeat("abcdefgh\n", 200) // 9 * 200 = 1800 bytes
	preview, truncated = previewSectionContent(long, 256)
	require.True(t, truncated)
	require.Contains(t, preview, "\n")
	require.Contains(t, preview, "(truncated, total")
	require.LessOrEqual(t, len(preview), 256+64,
		"截断后 preview 字节数应接近上限 + 注释长度")

	// 空内容路径
	preview, truncated = previewSectionContent("   \n  ", 100)
	require.False(t, truncated)
	require.Equal(t, "", preview)
}

// TestPromptSectionStatus_BytesPercentAndHash 单测新字段 BytesPercent / ContentHash
// 关键词: prompt_profile new fields test, BytesPercent, ContentHash, EstimatedTokens
func TestPromptSectionStatus_BytesPercentAndHash(t *testing.T) {
	a := NewPromptSectionObservation("k1", "L1", PromptSectionRoleSystemPrompt, false, "alpha\nbeta")
	b := NewPromptSectionObservation("k2", "L2", PromptSectionRoleRuntimeCtx, true, strings.Repeat("x", 4096))

	prompt := a.Content + "\n\n" + b.Content
	obs := BuildPromptObservation("loopX", "nonceX", prompt, []*PromptSectionObservation{a, b})
	status := obs.BuildStatus(0) // 0 -> 默认 4 KiB
	require.NotNil(t, status)
	require.Len(t, status.Sections, 2)

	s1 := status.Sections[0]
	s2 := status.Sections[1]
	require.Equal(t, "k1", s1.Key)
	require.Equal(t, "k2", s2.Key)

	// hash 必须 8 字符
	require.Len(t, s1.ContentHash, 8)
	require.Len(t, s2.ContentHash, 8)
	require.NotEqual(t, s1.ContentHash, s2.ContentHash)

	// EstimatedTokens 大于 0
	require.Greater(t, s1.EstimatedTokens, 0)
	require.Greater(t, s2.EstimatedTokens, 0)

	// BytesPercent 加起来近似 100 (考虑分隔符 "\n\n" = 2 字节, 不计入任何段)
	totalPct := s1.BytesPercent + s2.BytesPercent
	require.InDelta(t, 100, totalPct, 1.0,
		"两段字节占比之和应接近 100 (允许 1pp 抖动来源是 prompt 拼接分隔符)")

	// b 是 4KiB 占绝对多数
	require.Greater(t, s2.BytesPercent, s1.BytesPercent)
}
