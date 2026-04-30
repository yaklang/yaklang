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
		"<|PROMPT_SECTION_dynamic|>\n" + strings.TrimSpace(dynamic.Children[0].Content+"\n\n"+dynamic.Children[1].Content+"\n\n"+dynamic.Children[2].Content) + "\n<|PROMPT_SECTION_END_dynamic|>",
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

	loop.Set("prompt_observation_log", true)
}
