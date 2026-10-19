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
			var runtime *ReAct
			round := 0
			runtime, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
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
					response := cfg.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
					response.Close()
					return response, nil
				}))
			require.NoError(t, err)
			require.Nil(t, runtime.config.GetSkillLoader(), "the focus must work even when automatic Skill discovery is disabled")
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
