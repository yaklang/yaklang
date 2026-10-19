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
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestKnowledgeFocusContextReuseAndReadOnlyPolicy(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("read-only=%t", readOnly), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var tools []*aitool.Tool
			var executions, decisions int
			for _, name := range []string{"query_knowledge_base", "manage_knowledge_base"} {
				if readOnly && name == "manage_knowledge_base" {
					continue
				}
				tool, err := aitool.New(name, aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
					executions++
					return nil, nil
				}))
				require.NoError(t, err)
				tools = append(tools, tool)
			}
			older, err := aitool.New("older_cached_tool", aitool.WithDescription(strings.Repeat("older schema ", 2000)),
				aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
					executions++
					return nil, nil
				}))
			require.NoError(t, err)
			tools = append(tools, older)
			var runtime *ReAct
			round := 0
			runtime, err = NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
				aicommon.WithFocus(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEnableFunctionCallMode(false),
				aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tools...)),
				aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if req.GetCallerLabel() == "directly-answer" {
						response := cfg.NewAIResponse()
						response.EmitOutputStream(strings.NewReader(`{"@action":"directly_answer","answer_payload":"The selected knowledge preferences are available."}`))
						response.Close()
						return response, nil
					}
					require.Equal(t, "react-loop:knowledge_enhance", req.GetCallerLabel())
					decisions++
					prompt := req.GetPrompt()
					require.Contains(t, prompt, "本 Skill 统一描述 Yaklang 引擎")
					require.Equal(t, 1, strings.Count(prompt, "本 Skill 统一描述 Yaklang 引擎"), "do not duplicate the Skill body on re-entry")
					require.Contains(t, prompt, fmt.Sprintf("USER_PREFERENCE_%d", round))
					require.Contains(t, prompt, "attached_knowledge_base")
					manager := runtime.config.GetAiToolManager()
					require.True(t, manager.IsRecentlyUsedTool("query_knowledge_base"))
					require.Equal(t, !readOnly, manager.IsRecentlyUsedTool("manage_knowledge_base"), "preload must preserve the caller's tool allowlist")
					require.False(t, manager.IsRecentlyUsedTool(older.Name), "required focus schemas must displace an older working set when the cache is full")
					response := cfg.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
					response.Close()
					return response, nil
				}))
			require.NoError(t, err)
			require.Nil(t, runtime.config.GetSkillLoader(), "the focus must work even when automatic Skill discovery is disabled")
			mutation := runtime.config.RecordRecentlyUsedTool(older)
			require.NotNil(t, mutation.Upsert)
			buildinaitools.WithRecentToolCacheMaxTokens(mutation.Upsert.Size + 1)(runtime.config.GetAiToolManager())
			meta, ok := reactloops.GetLoopMetadata(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE)
			require.True(t, ok)
			require.False(t, meta.IsHidden)
			require.Equal(t, "知识增强检索", meta.VerboseNameZh)
			sink := aicommon.NewSkillContentTimelineAdapter(runtime.config.GetTimeline(), runtime.config.AcquireId)
			var skillParts int
			var schemas string
			for round = 0; round < 2; round++ {
				task := aicommon.NewStatefulTaskBase(fmt.Sprint("knowledge-", round), "Respect the attached knowledge preference.", ctx, runtime.Emitter)
				attachments := []*aicommon.AttachedResource{aicommon.NewAttachedResource(aicommon.AttachedResourceTypeKnowledgeBase, aicommon.CONTEXT_PROVIDER_KEY_NAME, fmt.Sprintf("USER_PREFERENCE_%d", round))}
				task.SetAttachedDatas(attachments)
				if round == 1 {
					// Cover both aim.focus/config and the frontend's task-level focus.
					runtime.config.Focus = schema.AI_REACT_LOOP_NAME_DEFAULT
					task.SetFocusMode(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE)
				}
				_, focus, opts := runtime.selectLoopForTask(task)
				require.Equal(t, schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, focus)
				runtime.SetCurrentTask(task)
				_, err := runtime.ExecuteLoopTask(focus, task, opts...)
				require.NoError(t, err)
				require.Equal(t, attachments, task.GetAttachedDatas())
				// The in-process model mock bypasses gateway projection. Commit its
				// pending context so the next task exercises reuse of frozen entries.
				runtime.config.GetTimeline().FreezeAll()
				view := aicommon.RenderTimelineFrozenOpen(runtime.config.GetTimeline())
				if round == 0 {
					skillParts = len(sink.VisibleSkillParts("yaklang-system"))
					require.Positive(t, skillParts)
					schemas = view.PromotedRecentTools
					require.NotEmpty(t, schemas)
				} else {
					require.Len(t, sink.VisibleSkillParts("yaklang-system"), skillParts)
					require.Equal(t, schemas, view.PromotedRecentTools, "re-entry must reuse the same cached schemas")
				}
			}
			require.Equal(t, 2, decisions, "no extra model turns for Skill or schema loading")
			require.Zero(t, executions, "preparing focus must not execute knowledge tools automatically")
		})
	}
}

func TestKnowledgeFocusPreservesCustomSkills(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	custom := filesys.NewVirtualFs()
	custom.AddFile("project-guide/SKILL.md", "---\nname: project-guide\ndescription: User project knowledge preferences\n---\nCUSTOM_KNOWLEDGE_PREFERENCE")
	runtime, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithSkillsFS(custom), aicommon.WithDisableCreateDBRuntime(true))
	require.NoError(t, err)
	loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, runtime)
	require.NoError(t, err)
	defer loop.Release()
	loader := loop.GetSkillsContextManager().GetLoader()
	project, err := loader.LoadSkill("project-guide")
	require.NoError(t, err, "adding the system Skill must preserve existing user sources")
	require.Contains(t, project.SkillMDContent, "CUSTOM_KNOWLEDGE_PREFERENCE")
	system, err := loader.LoadSkill("yaklang-system")
	require.NoError(t, err)
	require.Contains(t, system.SkillMDContent, "manage_knowledge_base")
	require.Contains(t, system.SkillMDContent, "query_knowledge_base")
	require.Len(t, loader.AllSkillMetas(), 2, "only the required system Skill should be added")
}
