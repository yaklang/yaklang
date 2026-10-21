package aireact

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

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

func TestKnowledgeFocusKeepsConfiguredSkillStartup(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("project-guide/SKILL.md", "---\nname: project-guide\ndescription: Custom guide\n---\nUSER_SELECTED_PROJECT_GUIDE")
	loader, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_FileSystem(GetBuiltinSkillsFS()), aiskillloader.WithAutoLoad_FileSystem(vfs))
	require.NoError(t, err)
	runtime, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithSkillLoader(loader), aicommon.WithEnabledCapabilities(aicommon.EnabledCapability{Name: "project-guide", Type: aicommon.EnabledCapabilityTypeSkill}))
	require.NoError(t, err)
	loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, runtime)
	require.NoError(t, err)
	defer loop.Release()
	require.Same(t, loader, loop.GetSkillsContextManager().GetLoader())
	require.True(t, loop.GetSkillsContextManager().IsSkillLoaded("project-guide"), "keep ordinary startup loading for user-selected Skills")

	manager := aiskillloader.NewSkillsContextManager(loader)
	other, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, runtime, reactloops.WithSkillsContextManager(manager))
	require.NoError(t, err)
	defer other.Release()
	require.Same(t, manager, other.GetSkillsContextManager(), "reuse an explicitly configured context manager")
}

func TestKnowledgeFocusRoutingAndReadOnlyPolicy(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("read-only=%t", readOnly), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var tools []*aitool.Tool
			executions, decisions, round := 0, 0, 0
			for _, name := range []string{"query_knowledge_base", "manage_knowledge_base"} {
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
				aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tools...)),
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
						require.Contains(t, prompt, "不要重复 loading_skills 或 require_tool")
						require.Contains(t, prompt, "库名必须原样使用完整值")
						require.Contains(t, prompt, "token_limit=1024、limit=3、per_entry_token_limit=256")
						require.True(t, runtime.config.GetAiToolManager().IsRecentlyUsedTool("query_knowledge_base"))
						require.Equal(t, !readOnly, runtime.GetCurrentLoop().Actions().Have(schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT))
						require.Equal(t, !readOnly, runtime.GetCurrentLoop().Actions().Have(schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION))
						response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
					}
					response.Close()
					return response, nil
				}))
			require.NoError(t, err)
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
