package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// Mock only model decisions and embeddings. Skill loading, require_tool,
// promoted schemas, action dispatch, embedded Yak scripts, functional options,
// runtime database binding, builds and CRUD all execute production code.
func TestKnowledgeBaseReActSkillToolsIntegration(t *testing.T) {
	oldMock := vectorstore.IsMockMode
	vectorstore.IsMockMode = true
	t.Cleanup(func() { vectorstore.IsMockMode = oldMock })
	for _, scenario := range []struct {
		focus  string
		native bool
	}{
		{schema.AI_REACT_LOOP_NAME_DEFAULT, false},
		{schema.AI_REACT_LOOP_NAME_DEFAULT, true},
		{schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, false},
		{schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, true},
	} {
		focus, native := scenario.focus, scenario.native
		t.Run(fmt.Sprintf("%s/native=%t", focus, native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			workdir := t.TempDir()
			profile, err := consts.CreateProfileDatabase(filepath.Join(workdir, "profile.db"))
			require.NoError(t, err)
			defer profile.Close()
			var outputs []string
			var tools []*aitool.Tool
			for _, name := range []string{"manage_knowledge_base", "query_knowledge_base"} {
				source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/knowledge/" + name + ".yak")
				require.NoError(t, err)
				metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
				require.NotNil(t, metadata)
				converted := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
				require.Len(t, converted, 1)
				tool := converted[0]
				callback := tool.Callback
				tool.Callback = func(ctx context.Context, params aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
					bound := &aitool.ToolRuntimeConfig{ProfileDatabase: profile}
					if runtime != nil {
						*bound = *runtime
						bound.ProfileDatabase = profile
					}
					var output bytes.Buffer
					result, err := callback(ctx, params, bound, io.MultiWriter(stdout, &output), stderr)
					outputs = append(outputs, output.String())
					return result, err
				}
				tools = append(tools, tool)
			}
			file := filepath.Join(workdir, "knowledge.txt")
			require.NoError(t, os.WriteFile(file, []byte("FILE_MARKER\nAPI requests require X-Knowledge-Token."), 0600))
			type step struct {
				tool   string
				params map[string]any
			}
			manage := func(op, name string, fields map[string]any) step {
				if fields == nil {
					fields = map[string]any{}
				}
				fields["op"], fields["name"] = op, name
				return step{"manage_knowledge_base", fields}
			}
			query := func(fields map[string]any) step {
				fields["token_limit"] = 512
				return step{"query_knowledge_base", fields}
			}
			steps := []step{
				manage("create", "docs", map[string]any{"metadata": map[string]any{"description": "project", "tags": []string{"fixture"}}, "index_options": map[string]any{"m": 8, "ef_search": 40, "ef_construct": 50}}),
				manage("create", "other", nil),
				manage("list", "", map[string]any{"limit": 1}),
				manage("get", "docs", nil),
				manage("update", "docs", map[string]any{"metadata": map[string]any{"description": "", "tags": []string{}, "is_default": false}}),
				manage("add_entry", "docs", map[string]any{"entry": map[string]any{"title": "API token", "content": "ORIGINAL_MARKER " + strings.Repeat("原文\"\\😀\n", 100)}}),
				query(map[string]any{"name": "docs", "query": "ORIGINAL_MARKER", "per_entry_token_limit": 256}),
				query(map[string]any{"name": "docs", "mode": "get", "entry_id": "pending", "content_offset": -1}),
				manage("update_entry", "docs", map[string]any{"entry_id": "pending", "entry": map[string]any{"title": "updated", "content": "NEW_MARKER API token"}}),
				manage("build", "docs", map[string]any{"source_file": file, "build_options": map[string]any{"mode": "chunks", "chunk_size": 512, "concurrency": 1}}),
				query(map[string]any{"names": []string{"docs", "other"}, "mode": "semantic", "query": "API token", "limit": 2}),
				query(map[string]any{"all": true, "query": "MARKER", "limit": 1}),
				query(map[string]any{"query": "MARKER"}), // Must fail, then the AI follows its guidance.
				manage("list", "", nil),
				query(map[string]any{"names": []string{"docs", "other"}, "query": "NEW_MARKER"}),
				manage("delete_entry", "docs", map[string]any{"entry_id": "pending"}),
				query(map[string]any{"name": "docs", "query": "NEW_MARKER"}),
				manage("delete", "docs", nil), manage("delete", "other", nil), manage("list", "", nil),
			}
			var runtime *aireact.ReAct
			var task aicommon.AIStatefulTask
			cursor, checked, requires := 0, 0, 0
			loaded := map[string]bool{}
			skillLoaded := false
			entryID, continuation := "", 0
			respond := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, name string, args map[string]any) (*aicommon.AIResponse, error) {
				response := c.NewAIResponse()
				wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
				if wire.ToolCallCallback != nil {
					raw, err := json.Marshal(args)
					require.NoError(t, err)
					wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprint("kb-", cursor, "-", requires), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(raw)}}})
					wire.FinishReasonCallback("tool_calls", nil)
				} else {
					args["@action"] = name
					raw, err := json.Marshal(args)
					require.NoError(t, err)
					response.EmitOutputStream(strings.NewReader(string(raw)))
				}
				response.Close()
				return response, nil
			}
			callback := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				if req.GetCallerLabel() == "verification" {
					return respond(c, req, "verify-satisfaction", map[string]any{"user_satisfied": cursor != 13, "reasoning": "Verify the actual knowledge tool outcome.", "evidence": []any{}})
				}
				if req.GetCallerLabel() == "directly-answer" {
					return respond(c, req, "directly_answer", map[string]any{"answer_payload": "Knowledge CRUD verified."})
				}
				if !strings.HasPrefix(req.GetCallerLabel(), "react-loop:") {
					return nil, fmt.Errorf("unexpected AI request: %s", req.GetCallerLabel())
				}
				prompt := req.GetPrompt()
				if focus == schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE {
					skillLoaded = true
					for _, name := range []string{"manage_knowledge_base", "query_knowledge_base"} {
						require.True(t, runtime.GetConfig().GetAiToolManager().IsRecentlyUsedTool(name), "focus must preload schemas before the first model decision")
						loaded[name] = true
					}
					for _, key := range []string{"attached_knowledge_base", "docs", "PREFERENCE_MARKER", "优先使用项目资料", "Raw Value"} {
						require.Contains(t, prompt, key, "user attachment preferences must reach the actual AI prompt")
					}
				}
				if !skillLoaded {
					require.Contains(t, prompt, "yaklang-system", "the actual Skill must appear in the catalog")
					skillLoaded = true
					return respond(c, req, "loading_skills", map[string]any{"skill_name": "yaklang-system"})
				}
				for _, key := range []string{"yaklang-system", "manage_knowledge_base", "query_knowledge_base", "token_limit", "all=true", "per_entry_token_limit"} {
					require.Contains(t, prompt, key, "the actual main prompt must contain the loaded Skill key %q", key)
				}
				wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
				for _, tool := range wire.Tools {
					require.NotEqual(t, "knowledge_enhance_answer", tool.Function.Name)
				}
				require.NotContains(t, prompt, "rewrite_user_query_for_knowledge_enhance")
				if checked < cursor {
					results := task.GetAllToolCallResults()
					require.Len(t, results, cursor)
					actual := results[cursor-1]
					require.Equal(t, steps[cursor-1].tool, actual.Name)
					if cursor == 13 {
						require.False(t, actual.Success)
						for _, key := range []string{"manage_knowledge_base", `"op":"list"`, "names", "all=true"} {
							require.Contains(t, actual.Error, key)
						}
					} else {
						require.True(t, actual.Success, "step %d: %+v", cursor, actual)
						require.Len(t, outputs, cursor)
						output := outputs[cursor-1]
						var value map[string]any
						require.NoError(t, json.Unmarshal([]byte(output), &value), output)
						if actual.Name == "query_knowledge_base" {
							require.LessOrEqual(t, ytoken.CalcTokenCount(output), 512)
						}
						switch cursor {
						case 1, 2:
							var count int
							require.NoError(t, profile.Model(&schema.KnowledgeBaseInfo{}).Count(&count).Error)
							require.Equal(t, cursor, count, "the script must use the runtime-bound profile")
						case 3:
							require.Equal(t, float64(2), value["total"])
							require.Equal(t, true, value["has_more"])
						case 4:
							require.Equal(t, float64(0), value["entry_count"])
						case 5:
							require.Equal(t, "", value["knowledge_base"].(map[string]any)["description"])
						case 6:
							entryID = value["entry_id"].(string)
						case 7:
							hit := value["results"].([]any)[0].(map[string]any)
							require.Equal(t, true, hit["truncated"])
							continuation = int(hit["next_content_offset"].(float64))
							require.Positive(t, continuation)
						case 8:
							require.Equal(t, float64(continuation), value["results"].([]any)[0].(map[string]any)["content_offset"])
						case 10:
							require.Greater(t, value["entries_added"].(float64), float64(0))
						case 11, 15:
							require.NotEmpty(t, value["results"])
						case 12:
							require.Len(t, value["results"], 1)
							require.Equal(t, float64(2), value["knowledge_base_count"])
						case 17, 20:
							require.Zero(t, value["total"])
						}
					}
					checked = cursor
				}
				if cursor == len(steps) {
					return respond(c, req, "finish", map[string]any{})
				}
				next := steps[cursor]
				manager := runtime.GetConfig().GetAiToolManager()
				if !loaded[next.tool] {
					require.False(t, manager.IsRecentlyUsedTool(next.tool), "require must cause schema promotion")
					loaded[next.tool] = true
					requires++
					return respond(c, req, "require_tool", map[string]any{"require_tool_payload": next.tool})
				}
				require.True(t, manager.IsRecentlyUsedTool(next.tool))
				contextSchema := prompt
				if native {
					raw, _ := json.Marshal(wire.Tools)
					contextSchema += string(raw)
				}
				keys := []string{"properties", "token_limit", "per_entry_token_limit", "content_offset", "names", "all"}
				if next.tool == "manage_knowledge_base" {
					keys = []string{"properties", "op", "metadata", "embedding_model", "dimension", "source_file", "build_options", "concurrency"}
				}
				for _, key := range keys {
					require.Contains(t, contextSchema, key, "the required tool's schema must reach AI context before execution")
				}
				if next.params["entry_id"] == "pending" {
					next.params["entry_id"] = entryID
				}
				if cursor == 7 {
					next.params["content_offset"] = continuation
				}
				cursor++
				return respond(c, req, "directly_call_tool", map[string]any{"directly_call_tool_name": next.tool, "directly_call_tool_params": next.params, "directly_call_reason": "Verify the knowledge tool integration"})
			}
			var skillOptions []aicommon.ConfigOption
			if focus == schema.AI_REACT_LOOP_NAME_DEFAULT {
				skillOptions = append(skillOptions, aicommon.WithSkillsFS(aireact.GetBuiltinSkillsFS()))
			}
			runtime, err = aireact.NewTestReAct(append(skillOptions, aicommon.WithContext(ctx), aicommon.WithEnableFunctionCallMode(native),
				aicommon.WithSingleAIModelMode(true), aicommon.WithDisableAutoSkills(true), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithAiToolManagerOptions(buildinaitools.WithExtendTools(tools, false)),
				aicommon.WithWorkdir(workdir), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithAgreeYOLO(),
				aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithAICallback(callback))...)
			require.NoError(t, err)
			task = aicommon.NewStatefulTaskBase("knowledge-integration", "Use the system Skill to manage and query knowledge bases.", ctx, runtime.GetConfig().GetEmitter(), true)
			attachments := []*aicommon.AttachedResource{
				aicommon.NewAttachedResource(aicommon.AttachedResourceTypeKnowledgeBase, aicommon.CONTEXT_PROVIDER_KEY_NAME, "docs"),
				aicommon.NewAttachedResource(aicommon.AttachedResourceTypeFile, aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT, "PREFERENCE_MARKER 优先使用项目资料；保留这段用户附加说明。"),
			}
			if focus == schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE {
				task.SetAttachedDatas(attachments)
			}
			runtime.SetCurrentTask(task)
			loop, err := reactloops.CreateLoopByName(focus, runtime, reactloops.WithMaxIterations(32))
			require.NoError(t, err)
			require.Equal(t, focus, loop.GetLoopName())
			defer loop.Release()
			require.NoError(t, loop.ExecuteWithExistedTask(task))
			require.Equal(t, len(steps), checked)
			if focus == schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE {
				require.Zero(t, requires, "the focus must not spend model turns on Skill or schema loading")
				require.Equal(t, attachments, task.GetAttachedDatas(), "initialization must preserve original attachments")
			} else {
				require.Equal(t, 2, requires)
			}
			require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
			var count int
			require.NoError(t, profile.Model(&schema.KnowledgeBaseInfo{}).Count(&count).Error)
			require.Zero(t, count, "all fixture bases must be deleted through real tool calls")
		})
	}
}
