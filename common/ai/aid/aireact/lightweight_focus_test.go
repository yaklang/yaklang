package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestLightweightFocusPinnedContext(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, speed := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%t/speed=%t", native, speed), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				vfs := filesys.NewVirtualFs()
				vfs.AddFile("test-focus/SKILL.md", "---\nname: test-focus\ndescription: Focus test\nmetadata:\n  auto_load: \"true\"\n---\nPINNED_SKILL_BODY\n")
				loader, err := aiskillloader.NewFSSkillLoader(vfs)
				require.NoError(t, err)
				var runtime *ReAct
				var loop *reactloops.ReActLoop
				calls, decisions, compressions := 0, 0, 0
				pressure, err := aitool.New("cache_pressure", aitool.WithStringParam("ordinary_schema"), aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return nil, nil }))
				require.NoError(t, err)
				probe, err := aitool.New("focus_probe", aitool.WithStringParam("PINNED_SCHEMA_PARAM", aitool.WithParam_Required(true)),
					aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
						calls++
						require.Equal(t, "ok", params.GetString("PINNED_SCHEMA_PARAM"))
						// Exercise the real LRU, explicit Skill retirement and actual
						// Timeline compression between two decisions of the same loop.
						runtime.config.RecordRecentlyUsedTool(pressure)
						loop.GetSkillsContextManager().UnloadSkill("test-focus")
						runtime.config.Timeline.PushText(runtime.config.AcquireId(), strings.Repeat("ORDINARY_HISTORY_TO_REDUCE ", 300))
						compressed, err := runtime.config.Timeline.CompressOnce(aicommon.TimelineCompressionOptions{Context: ctx, MaxInputTokens: 100000, MaxSummaryTokens: 1024})
						require.NoError(t, err)
						require.Equal(t, "HISTORY_WAS_COMPRESSED", compressed.Summary)
						require.False(t, runtime.config.GetAiToolManager().IsRecentlyUsedTool("focus_probe"))
						require.True(t, loop.HasToolSchema("focus_probe"))
						return "PROBE_EXECUTED", nil
					}))
				require.NoError(t, err)
				respond := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, name string, args map[string]any) (*aicommon.AIResponse, error) {
					response := c.NewAIResponse()
					wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					if wire.ToolCallCallback != nil {
						raw, _ := json.Marshal(args)
						wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprint("focus-", decisions), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(raw)}}})
						wire.FinishReasonCallback("tool_calls", nil)
					} else {
						args["@action"] = name
						raw, _ := json.Marshal(args)
						response.EmitOutputStream(strings.NewReader(string(raw)))
					}
					response.Close()
					return response, nil
				}
				callback := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					switch req.GetCallerLabel() {
					case aicommon.CallerLabelTimelineCompress:
						compressions++
						require.NotContains(t, req.GetPrompt(), "PINNED_SKILL_BODY")
						require.NotContains(t, req.GetPrompt(), "## Tool: focus_probe")
						return respond(c, req, "timeline-summary", map[string]any{"summary": "HISTORY_WAS_COMPRESSED", "ratain_timeline_item_range": "", "memory_entities": []any{}})
					case "directly-answer":
						return respond(c, req, "directly_answer", map[string]any{"answer_payload": "Probe verified."})
					case "verification":
						return respond(c, req, "verify-satisfaction", map[string]any{"user_satisfied": true, "reasoning": "Probe verified", "evidence": []any{}})
					}
					require.True(t, strings.HasPrefix(req.GetCallerLabel(), "react-loop:"), req.GetCallerLabel())
					decisions++
					prompt := req.GetPrompt()
					require.Contains(t, prompt, "HOST_STATIC_OVERRIDE")
					require.NotContains(t, prompt, "REPLACED_STATIC_INSTRUCTION")
					require.Equal(t, 1, strings.Count(prompt, "PINNED_SKILL_BODY"))
					require.Equal(t, 1, strings.Count(prompt, "## Tool: focus_probe"))
					require.Contains(t, prompt, "PINNED_SCHEMA_PARAM")
					require.Contains(t, prompt, "directly_call_tool")
					require.Less(t, strings.Index(prompt, "HOST_STATIC_OVERRIDE"), strings.Index(prompt, "# Pinned Focus Context"), "host rules belong to the stronger static section")
					if calls == 0 {
						return respond(c, req, "directly_call_tool", map[string]any{"directly_call_tool_name": "focus_probe", "directly_call_tool_params": map[string]any{"PINNED_SCHEMA_PARAM": "ok"}})
					}
					return respond(c, req, "finish", map[string]any{})
				}
				runtime, err = NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithEnableFunctionCallMode(native), aicommon.WithSingleAIModelMode(true), aicommon.WithAgreeYOLO(),
					aicommon.WithAICallback(callback), aicommon.WithSpeedPriorityAICallback(callback),
					aicommon.WithNoOpMemoryTriage(), aicommon.WithDisableToolCallerIntervalReview(true),
					aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(probe, pressure), buildinaitools.WithRecentToolCacheMaxTokens(1)))
				require.NoError(t, err)
				name := fmt.Sprintf("lightweight_test_%d", time.Now().UnixNano())
				require.NoError(t, loop_default.RegisterLightweightFocus(name, []reactloops.ReActLoopOption{
					reactloops.WithSkillLoader(loader), reactloops.WithPinnedSkills("test-focus", "test-focus"),
					reactloops.WithPinnedTools("focus_probe", "focus_probe"), reactloops.WithStaticInstruction("REPLACED_STATIC_INSTRUCTION"),
				}))
				loop, err = reactloops.CreateLoopByName(name, runtime, reactloops.WithStaticInstruction("HOST_STATIC_OVERRIDE"), reactloops.WithUseSpeedPriorityAICallback(speed), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true))
				require.NoError(t, err)
				defer loop.Release()
				task := aicommon.NewStatefulTaskBase("focus-probe", "Run the pinned probe and verify its result.", ctx, runtime.Emitter, true)
				runtime.SetCurrentTask(task)
				require.Zero(t, calls, "construction only loads definitions")
				require.NoError(t, loop.ExecuteWithExistedTask(task))
				require.Equal(t, 1, calls)
				require.Equal(t, 1, compressions)
				require.Equal(t, 2, decisions, "no model turns for loading Skills or tools")
				// Leaving this focus must not leak its pinned roots or host rules
				// into another ordinary loop on the same session.
				ordinary, err := loop_default.NewLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, runtime)
				require.NoError(t, err)
				defer ordinary.Release()
				require.False(t, ordinary.HasPinnedContext())
				require.False(t, ordinary.HasToolSchema("focus_probe"))
			})
		}
	}
}

func TestLightweightFocusRejectsUnavailableOrOversizedContext(t *testing.T) {
	runtime, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools()))
	require.NoError(t, err)
	for _, option := range []reactloops.ReActLoopOption{
		reactloops.WithPinnedTools("disabled-tool"),
		reactloops.WithPinnedSkills("missing-skill"),
		reactloops.WithStaticInstruction(strings.Repeat("oversized static instruction ", 20000)),
	} {
		_, err := loop_default.NewLoop("invalid-focus", runtime, option)
		require.Error(t, err)
	}
}

func TestKnowledgeFocusPreservesCustomSkillLoader(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("project-guide/SKILL.md", "---\nname: project-guide\ndescription: Custom guide\n---\nCUSTOM_SELECTION_PREFERENCE")
	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	require.NoError(t, err)
	runtime, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true))
	require.NoError(t, err)
	loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, runtime, reactloops.WithSkillLoader(loader))
	require.NoError(t, err)
	defer loop.Release()
	combined := loop.GetSkillsContextManager().GetLoader()
	custom, err := combined.LoadSkill("project-guide")
	require.NoError(t, err)
	require.Contains(t, custom.SkillMDContent, "CUSTOM_SELECTION_PREFERENCE")
	require.Len(t, combined.AllSkillMetas(), 2)
	require.Len(t, loader.AllSkillMetas(), 1, "the caller's loader is not mutated")
}

func TestKnowledgeFocusRoutingAndReadOnlyPolicy(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("read-only=%t", readOnly), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var tools []*aitool.Tool
			executions, decisions, round := 0, 0, 0
			for _, name := range []string{"query_knowledge_base", "manage_knowledge_base", "older_cached_tool"} {
				if readOnly && name == "manage_knowledge_base" {
					continue
				}
				tool, err := aitool.New(name, aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { executions++; return nil, nil }))
				require.NoError(t, err)
				tools = append(tools, tool)
			}
			var runtime *ReAct
			runtime, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithFocus(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE), aicommon.WithEnableFunctionCallMode(false),
				aicommon.WithEnablePlanAndExec(!readOnly),
				aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tools...), buildinaitools.WithRecentToolCacheMaxTokens(1)),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					response := c.NewAIResponse()
					if req.GetCallerLabel() == "directly-answer" {
						response.EmitOutputStream(strings.NewReader(`{"@action":"directly_answer","answer_payload":"Preferences preserved."}`))
					} else {
						require.Equal(t, "react-loop:knowledge_enhance", req.GetCallerLabel())
						decisions++
						prompt := req.GetPrompt()
						require.Equal(t, 1, strings.Count(prompt, "本 Skill 统一描述 Yaklang 引擎"))
						require.Equal(t, 1, strings.Count(prompt, "## Tool: query_knowledge_base"))
						require.Equal(t, !readOnly, strings.Contains(prompt, "## Tool: manage_knowledge_base"))
						require.Contains(t, prompt, fmt.Sprintf("USER_PREFERENCE_%d", round))
						require.Contains(t, prompt, "attached_knowledge_base")
						require.True(t, runtime.GetCurrentLoop().HasToolSchema("query_knowledge_base"))
						require.Equal(t, !readOnly, runtime.GetCurrentLoop().Actions().Have(schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT))
						require.Equal(t, !readOnly, runtime.GetCurrentLoop().Actions().Have(schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION))
						require.True(t, runtime.config.GetAiToolManager().IsRecentlyUsedTool("older_cached_tool"), "pinning must not evict ordinary schemas")
						response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
					}
					response.Close()
					return response, nil
				}))
			require.NoError(t, err)
			runtime.config.RecordRecentlyUsedTool(tools[len(tools)-1])
			meta, ok := reactloops.GetLoopMetadata(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE)
			require.True(t, ok)
			require.False(t, meta.IsHidden)
			require.Equal(t, "知识增强检索", meta.VerboseNameZh)
			for round = 0; round < 2; round++ {
				task := aicommon.NewStatefulTaskBase(fmt.Sprint("preference-", round), "Respect my attached knowledge preference.", ctx, runtime.Emitter)
				attached := []*aicommon.AttachedResource{aicommon.NewAttachedResource(aicommon.AttachedResourceTypeKnowledgeBase, aicommon.CONTEXT_PROVIDER_KEY_NAME, fmt.Sprintf("USER_PREFERENCE_%d", round))}
				task.SetAttachedDatas(attached)
				if round == 1 {
					runtime.config.Focus = schema.AI_REACT_LOOP_NAME_DEFAULT
					task.SetFocusMode(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE)
				}
				_, focus, opts := runtime.selectLoopForTask(task)
				require.Equal(t, schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, focus)
				runtime.SetCurrentTask(task)
				_, err = runtime.ExecuteLoopTask(focus, task, opts...)
				require.NoError(t, err)
				require.Equal(t, attached, task.GetAttachedDatas())
				runtime.config.Timeline.FreezeAll()
			}
			require.Equal(t, 2, decisions)
			require.Zero(t, executions)
		})
	}
}
