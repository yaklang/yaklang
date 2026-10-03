package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Only the model is scripted. Read-file, Evidence, DAG preparation, action
// parsing, prompt assembly and manual approval all use production channels.
// Sample immediately before submit_plan, after inspect_plan exposed the draft.
func TestCoordinatorPlanningSubmissionSmoke(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, source := range []string{"exploration", "preset", "mocker"} {
			t.Run(fmt.Sprintf("%s/function_call_%v", source, native), func(t *testing.T) {
				runPlanningSubmissionSmoke(t, native, source)
			})
		}
	}
}

func runPlanningSubmissionSmoke(t *testing.T, native bool, source string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	workdir := t.TempDir()
	sourcePath := filepath.Join(workdir, "source.txt")
	const observation = "coordinator-planning-smoke: 审核一次后执行；第二个任务依赖第一个任务验收。"
	require.NoError(t, os.WriteFile(sourcePath, []byte(observation), 0600))
	const query = "先核对 source.txt，制定两个有依赖的只读任务并提交计划供我审核。本次只验证提交，先不执行任务。"
	const constraint = "补充约束：不修改源码，不执行命令，不访问外部网络。"
	const document = "# 来源核对计划\n只读核对 source.txt 并保存 Evidence；前置任务验收后，后继任务引用其证据生成 Markdown 报告。"
	const evidenceID = "planning.source.contract"
	plan := map[string]any{
		"main_task": "来源核对", "main_task_goal": "核对来源并产出有依据的报告", "main_task_identifier": "source_review",
		"tasks": []any{map[string]any{
			"subtask_name": "只读检查组", "subtask_goal": "组织来源核对和报告两个叶任务", "subtask_identifier": "checks",
			"sub_subtasks": []any{
				map[string]any{"subtask_name": "核对来源", "subtask_goal": "读取 source.txt 并保存实际观测为 session Evidence", "subtask_identifier": "read_source", "depends_on": []string{}},
				map[string]any{"subtask_name": "形成报告", "subtask_goal": "引用前置任务已验收的 Evidence，在 artifacts 中形成 Markdown 报告", "subtask_identifier": "report", "depends_on": []string{"read_source"}},
			},
		}},
	}
	rawPlan, err := json.Marshal(plan)
	require.NoError(t, err)
	parsed, err := coordinator.ParsePlan(string(rawPlan), document, nil)
	require.NoError(t, err)
	var root coordinator.PlanNode
	require.NoError(t, json.Unmarshal(parsed.Tree, &root))
	var s *coordinator.Session
	var actions []string
	var submitPrompt, approvedPrompt string
	var submitTools []aispec.Tool
	var submitMessages []aispec.ChatDetail
	mockerCalls := 0
	var eventsMu sync.Mutex
	var approvals []json.RawMessage
	var approvalErrors []error
	input := make(chan *ypb.AIInputEvent, 4)
	options := []aicommon.ConfigOption{
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithGenerateReport(false),
		aicommon.WithEnableFunctionCallMode(native), aicommon.WithWorkdir(workdir), aicommon.WithAgreeYOLO(),
		aicommon.WithForceManualPlanReview(true), aicommon.WithEventInputChan(input),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.Type != schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				return
			}
			eventsMu.Lock()
			defer eventsMu.Unlock()
			approvals = append(approvals, append(json.RawMessage(nil), e.Content...))
			var payload struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(e.Content, &payload); err != nil || payload.ID == "" {
				approvalErrors = append(approvalErrors, fmt.Errorf("invalid plan review payload: %s", e.Content))
				cancel()
				return
			}
			// The same continue payload supplied by the existing review UI.
			select {
			case input <- &ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: payload.ID, InteractiveJSONInput: `{"suggestion":"continue"}`}:
			case <-ctx.Done():
				approvalErrors = append(approvalErrors, ctx.Err())
			}
		}),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if req.GetCallerLabel() != "react-loop:coordinator" {
				return nil, fmt.Errorf("unexpected auxiliary or worker request: %s", req.GetCallerLabel())
			}
			if len(actions) > 8 {
				return nil, fmt.Errorf("planning submission did not converge: %v", actions)
			}
			prompt := req.GetPrompt()
			state := s.Snapshot()
			respond := func(name string, args map[string]any) (*aicommon.AIResponse, error) {
				actions = append(actions, name)
				return protocolResponse(c, req, native, name, args)
			}
			if source == "exploration" && state.Draft == nil {
				switch len(actions) {
				case 0:
					return respond("directly_call_tool", map[string]any{
						"directly_call_tool_name": "read_file", "directly_call_tool_params": map[string]any{"file": sourcePath},
						"directly_call_reason": "先读取真实来源，依据观测制定计划。",
					})
				case 1:
					// This sentinel is present only in the actual read-file output.
					require.Contains(t, prompt, observation, "read_file must execute before saving evidence")
					return respond("save_evidence", map[string]any{"evidence_id": evidenceID, "evidence_content": "主体：source.txt；方式：read_file 读取本地文件；观测：" + observation + "；决策：计划使用两项叶任务和明确的验收依赖。"})
				default:
					require.Contains(t, s.GetSessionEvidenceRendered(), evidenceID)
					// Exercise the existing Open -> frozen -> SemiDynamic1 promotion;
					// no extra model call and no custom prompt material injection.
					s.Timeline.FreezeAll()
					return respond("create_plan", map[string]any{"plan": plan, "plan_document": document})
				}
			}
			if state.Approved == nil {
				if len(actions) == 0 || actions[len(actions)-1] == "create_plan" {
					return respond("inspect_plan", map[string]any{})
				}
				submitPrompt = prompt
				wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
				submitTools = wire.Tools
				submitMessages = aiprojection.Project(aiprojection.ProjectionInput{Prompt: prompt, ActionTools: wire.Tools}).Messages
				return respond("submit_plan", map[string]any{"plan_version": state.DraftVersion})
			}
			approvedPrompt = prompt
			return respond("finish", map[string]any{})
		}),
	}
	switch source {
	case "preset":
		options = append(options, coordinator.WithPresetPlan(string(rawPlan), document))
	case "mocker":
		options = append(options, coordinator.WithPlanMocker(func(*coordinator.Session) *coordinator.PlanResponse {
			mockerCalls++
			return &coordinator.PlanResponse{RootTask: &root, Document: document}
		}))
	}
	s, err = coordinator.NewSession(ctx, query, options...)
	require.NoError(t, err)
	defer s.Close()
	_, err = s.AppendUserInputHistory(constraint, time.Now())
	require.NoError(t, err)
	s.Timeline.PushText(s.AcquireId(), "环境观测：当前工作区 source.txt 已由测试夹具创建；尚未派发任务。")
	if source != "exploration" {
		// An attached plan may arrive with prior session evidence. Verify a real
		// local source here; this is fixture input, not coordinator exploration.
		content, err := os.ReadFile(sourcePath)
		require.NoError(t, err)
		evidence, err := aicommon.BuildSessionEvidenceUpsert(evidenceID, "已有会话观测：测试夹具读取 source.txt，内容为 "+string(content))
		require.NoError(t, err)
		s.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{evidence})
	}
	s.Timeline.FreezeAll()
	require.NoError(t, s.RunPlanOnly())

	state := s.Snapshot()
	require.EqualValues(t, 1, state.DraftVersion)
	require.EqualValues(t, 1, state.ApprovedVersion)
	require.NotNil(t, state.Approved)
	require.Equal(t, document, state.Approved.Document)
	require.Len(t, state.Approved.Tasks, 2, "structural parent must not be dispatched")
	first, second := state.Approved.Tasks[0], state.Approved.Tasks[1]
	require.Equal(t, "核对来源", first.Name)
	require.Equal(t, "形成报告", second.Name)
	var approvedRoot coordinator.PlanNode
	require.NoError(t, json.Unmarshal(state.Approved.Tree, &approvedRoot))
	require.Equal(t, "read_source", approvedRoot.Subtasks[0].Subtasks[0].Identifier)
	require.Equal(t, "report", approvedRoot.Subtasks[0].Subtasks[1].Identifier)
	require.Equal(t, []string{first.ID}, second.DependsOn)
	for _, attempt := range state.Attempts {
		require.Zero(t, attempt.ID, "this smoke stops after submission, without a worker")
		require.Equal(t, coordinator.Pending, attempt.State)
	}
	expected := []string{"inspect_plan", "submit_plan", "finish"}
	if source == "exploration" {
		expected = append([]string{"directly_call_tool", "save_evidence", "create_plan"}, expected...)
	}
	require.Equal(t, expected, actions, "no hidden planning/review model calls")
	if source == "mocker" {
		require.Equal(t, 1, mockerCalls)
	}
	eventsMu.Lock()
	require.Empty(t, approvalErrors)
	require.Len(t, approvals, 1, "submit_plan requires exactly one manual confirmation")
	var approval map[string]any
	require.NoError(t, json.Unmarshal(approvals[0], &approval))
	eventsMu.Unlock()
	require.Equal(t, true, approval["force_manual_review"])
	require.NotEmpty(t, approval["selectors"])
	require.NotEmpty(t, approval["plans_id"])
	require.NotEmpty(t, approval["plans"])
	for _, prompt := range []string{submitPrompt, approvedPrompt} {
		require.Contains(t, prompt, "你是计划协调员")
		require.Contains(t, prompt, "# PLAN DEFINITION")
		require.Contains(t, prompt, "只读检查组")
		require.Contains(t, prompt, evidenceID)
		require.Contains(t, prompt, constraint)
		require.Equal(t, 1, strings.Count(prompt, query), "user input appears once, only in Timeline or promoted history")
		dynamic := regexp.MustCompile(`(?s)<\|PROMPT_SECTION_dynamic_[^|]+\|>(.*?)<\|PROMPT_SECTION_dynamic_END_`).FindStringSubmatch(prompt)
		require.Len(t, dynamic, 2, "missing Dynamic section")
		require.NotContains(t, dynamic[1], query)
		require.NotContains(t, dynamic[1], constraint)
		require.Less(t, strings.Index(prompt, "# Timeline Memory (Open Tail)"), strings.Index(prompt, "# PLAN STATUS"))
		require.Less(t, strings.Index(prompt, "# PLAN STATUS"), strings.Index(prompt, "## 待办清单（TODO）"))
	}
	require.Contains(t, submitPrompt, "Draft version: 1; approved version: 0")
	require.Contains(t, submitPrompt, `"draft":{`, "inspect_plan must expose complete draft before submission")
	require.Contains(t, approvedPrompt, "# PLAN DOCUMENT")
	require.Contains(t, approvedPrompt, "Approved PLAN version 1")
	require.Contains(t, approvedPrompt, "Dispatch: blocked")
	if native {
		require.Contains(t, submitPrompt, "本轮使用原生 function call：")
		require.NotContains(t, submitPrompt, "本轮使用文本流 JSON action：")
		seen := map[string]bool{}
		for _, tool := range submitTools {
			seen[tool.Function.Name] = true
		}
		for _, name := range coordinator.ActionNames {
			require.True(t, seen[name], "missing native action %s", name)
		}
	} else {
		require.Empty(t, submitTools)
		require.Contains(t, submitPrompt, "本轮使用文本流 JSON action：")
		require.NotContains(t, submitPrompt, "本轮使用原生 function call：")
		require.Contains(t, submitPrompt, "文本流模式通过 @action")
	}
	// Preset and mocker run independently with the same plan/document/DAG
	// assertions. Export preset as the representative attached-plan sample:
	// two scenarios x two protocols, exactly four primary prompt files.
	if dir := os.Getenv("COORDINATOR_CONTEXT_REVIEW_DIR"); dir != "" && source != "mocker" {
		protocol := "text-stream"
		if native {
			protocol = "function-call"
		}
		dir = filepath.Join(dir, "planning-submission")
		require.NoError(t, os.MkdirAll(dir, 0700))
		stem := source + "-" + protocol
		require.NoError(t, os.WriteFile(filepath.Join(dir, stem+".prompt.txt"), []byte(submitPrompt), 0600))
		sample, err := json.MarshalIndent(map[string]any{
			"scenario": source, "protocol": protocol, "sample_stage": "after inspect_plan; immediately before submit_plan",
			"model":   "deterministic fixture; real coordinator, tools, Timeline and manual approval",
			"actions": actions, "prompt_bytes": len(submitPrompt), "messages": submitMessages, "tools": submitTools,
			"manual_confirmation_count": len(approvals), "plan_review_payload": approval, "approved_plan": state.Approved,
		}, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, stem+".request.json"), sample, 0600))
	}
	t.Logf("%s native=%v: actions=%v; confirmation=1; approved leaves=2; worker attempts=0; submit prompt=%d bytes; native tools=%d", source, native, actions, len(submitPrompt), len(submitTools))
}
