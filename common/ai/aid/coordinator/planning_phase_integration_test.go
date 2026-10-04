package coordinator_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var planDefinitionJSON = regexp.MustCompile("(?s)# PLAN DEFINITION\\n.*?```json\\n(.*?)\\n```")

func currentDefinition(prompt string) (*coordinator.PlanNode, error) {
	match := planDefinitionJSON.FindStringSubmatch(prompt)
	if len(match) != 2 {
		return nil, fmt.Errorf("current PLAN DEFINITION missing")
	}
	var root coordinator.PlanNode
	err := json.Unmarshal([]byte(match[1]), &root)
	return &root, err
}

func definitionNode(root *coordinator.PlanNode, identifier string) *coordinator.PlanNode {
	if root.Identifier == identifier {
		return root
	}
	for _, n := range root.Subtasks {
		if found := definitionNode(n, identifier); found != nil {
			return found
		}
	}
	return nil
}

func TestPlanPhaseIntegration(t *testing.T) {
	for _, source := range []string{"investigation", "preset", "mocker"} {
		for _, native := range []bool{true, false} {
			for _, explore := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/native=%t/explore=%t", source, native, explore), func(t *testing.T) { runPlanPhaseIntegration(t, source, native, explore) })
			}
		}
	}
}

func runPlanPhaseIntegration(t *testing.T, source string, native, explore bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	workdir := t.TempDir()
	fixture := filepath.Join(workdir, "source.txt")
	observation := source + "-source-confirmed: 审核入口与执行入口相互独立。"
	require.NoError(t, os.WriteFile(fixture, []byte(observation), 0600))
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}, &schema.AiCheckpoint{}, &schema.AISession{}, &schema.AIAgentRuntime{}).Error)
	query := "核对本地来源，维护包含任务组和依赖的计划，提交一次审核；本次停在规划移交。"
	plan := map[string]any{"main_task": "来源检查", "main_task_goal": "根据真实证据制定可审阅计划", "main_task_identifier": "root", "tasks": []any{map[string]any{"subtask_name": "检查组", "subtask_goal": "组织来源读取与复核", "subtask_identifier": "checks", "sub_subtasks": []any{
		map[string]any{"subtask_name": "来源读取", "subtask_goal": "核对文件并保存依据", "subtask_identifier": "read", "depends_on": []string{}},
		map[string]any{"subtask_name": "来源复核", "subtask_goal": "依据前置证据复核", "subtask_identifier": "verify", "depends_on": []string{"read"}},
	}}}}
	rawPlan, _ := json.Marshal(plan)
	initialDoc := "# 初始来源计划\n先核对来源。\n"
	updatedDoc := "# 检查计划\n先核对来源。\n"
	finalDoc := updatedDoc + "记录来源路径与验证依据。\n"
	patch := "--- plan_document.md\n+++ plan_document.md\n@@ -1,2 +1,3 @@\n # 检查计划\n 先核对来源。\n+记录来源路径与验证依据。\n"
	var mainCalls, childCalls, auxiliaryCalls, waits, approvals, mockerCalls atomic.Int64
	var mu sync.Mutex
	var trace []string
	var reviewPayload map[string]any
	var coordinatorID string
	record := func(msg string) { mu.Lock(); trace = append(trace, msg); mu.Unlock() }
	waiting := make(chan struct{}, 4)
	discoveryRead := make(chan struct{})
	var discoveryOnce sync.Once
	assertSleeping := func() error {
		select {
		case <-waiting:
		case <-ctx.Done():
			return ctx.Err()
		}
		before := mainCalls.Load()
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		if mainCalls.Load() != before {
			return fmt.Errorf("coordinator polled while waiting: %d -> %d", before, mainCalls.Load())
		}
		record("自动挂起 50ms：主模型请求未增加")
		return nil
	}
	type step struct {
		name string
		args func(string) (map[string]any, error)
	}
	constant := func(v map[string]any) func(string) (map[string]any, error) {
		return func(string) (map[string]any, error) { return v, nil }
	}
	var steps []step
	steps = append(steps, step{"directly_call_tool", constant(map[string]any{"directly_call_tool_name": "read_file", "directly_call_tool_params": map[string]any{"file": fixture}, "directly_call_reason": "调查真实文件内容"})}, step{"save_evidence", func(prompt string) (map[string]any, error) {
		if !strings.Contains(prompt, observation) {
			return nil, fmt.Errorf("real read_file result missing")
		}
		return map[string]any{"evidence_id": "phase.source", "evidence_content": observation + "；来源：" + fixture}, nil
	}})
	if source == "investigation" {
		steps = append(steps, step{"create_plan", constant(map[string]any{"plan": plan, "plan_document": initialDoc})})
	}
	if explore {
		steps = append(steps, step{"dispatch_sub_react_agents", constant(map[string]any{"dispatches": []any{map[string]any{"identifier": "investigator", "goal": "只读调查 " + fixture + "，验证审核入口与执行入口的区别。禁止写入、命令和递归派发。", "result_contract": "保存共享 Evidence，交付来源与结论。", "context_mode": "fork"}}})},
			step{"submit_plan", func(prompt string) (map[string]any, error) {
				if !strings.Contains(prompt, "phase.discovery") {
					return nil, fmt.Errorf("discovery notification preceded shared evidence")
				}
				discoveryOnce.Do(func() { close(discoveryRead) })
				return map[string]any{}, nil
			}})
	}
	steps = append(steps, step{"modify_plan", constant(map[string]any{"document": updatedDoc})}, step{"modify_plan", constant(map[string]any{"tasks": plan})},
		step{"modify_plan", func(prompt string) (map[string]any, error) {
			root, err := currentDefinition(prompt)
			if err != nil {
				return nil, err
			}
			return map[string]any{"document_patch": patch, "tasks_patch": []any{map[string]any{"operator": "update", "task_id": definitionNode(root, "verify").TaskID, "changes": map[string]any{"subtask_goal": "依据来源路径和前置证据复核"}}}}, nil
		}},
		step{"modify_plan", constant(map[string]any{"tasks_patch": []any{map[string]any{"operator": "add", "task": map[string]any{"subtask_name": "临时检查", "subtask_goal": "验证操作批次", "subtask_identifier": "temporary"}}}})},
		step{"modify_plan", func(prompt string) (map[string]any, error) {
			root, err := currentDefinition(prompt)
			if err != nil {
				return nil, err
			}
			return map[string]any{"document": "ROLLBACK_SENTINEL", "tasks_patch": []any{map[string]any{"operator": "update", "task_id": definitionNode(root, "verify").TaskID, "changes": map[string]any{"depends_on": []string{"missing"}}}}}, nil
		}},
		step{"modify_plan", func(prompt string) (map[string]any, error) {
			projection := aiprojection.Project(aiprojection.ProjectionInput{Prompt: prompt})
			if strings.Contains(fmt.Sprint(projection.Messages[2].Content), "ROLLBACK_SENTINEL") {
				return nil, fmt.Errorf("failed transaction leaked candidate document")
			}
			return map[string]any{"document": finalDoc}, nil
		}},
		step{"modify_plan", func(prompt string) (map[string]any, error) {
			root, err := currentDefinition(prompt)
			if err != nil {
				return nil, err
			}
			return map[string]any{"tasks_patch": []any{
				map[string]any{"operator": "delete", "task_id": definitionNode(root, "temporary").TaskID},
				map[string]any{"operator": "update", "task_id": definitionNode(root, "read").TaskID, "changes": map[string]any{"subtask_identifier": "read_checked"}},
				map[string]any{"operator": "update", "task_id": definitionNode(root, "verify").TaskID, "changes": map[string]any{"depends_on": []string{"read_checked"}}},
			}}, nil
		}},
		step{"save_evidence", constant(map[string]any{"evidence_id": "phase.cache", "evidence_content": "相同发现不重复更新计划正文。"})}, step{"save_evidence", constant(map[string]any{"evidence_id": "phase.cache", "evidence_content": "相同发现不重复更新计划正文。"})},
		step{"submit_plan", constant(map[string]any{})}, step{"finish", constant(map[string]any{})})
	index := 0
	var originalIDs map[string]string
	var previousDefinition, previousDocument string
	var toolHash [32]byte
	var stableSections [3][32]byte
	var cachePrefix [4][32]byte
	var stableComparisons int
	var minimumReuse = 1.0
	reviewDir := os.Getenv("COORDINATOR_CONTEXT_REVIEW_DIR")
	if reviewDir != "" {
		reviewDir = filepath.Join(reviewDir, "planning-phase", fmt.Sprintf("%s-native-%t-explore-%t", source, native, explore))
		require.NoError(t, os.MkdirAll(reviewDir, 0700))
	}
	model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		if req.GetCallerLabel() == "verification" {
			auxiliaryCalls.Add(1)
			resp := cfg.NewAIResponse()
			resp.EmitOutputStream(strings.NewReader(`{"@action":"verify-satisfaction","user_satisfied":false,"reasoning":"来源已读；还需保存共享证据并交付调查结论。","evidence":[]}`))
			resp.Close()
			return resp, nil
		}
		if req.GetCallerLabel() == "directly-answer" {
			auxiliaryCalls.Add(1)
			resp := cfg.NewAIResponse()
			resp.EmitOutputStream(strings.NewReader("调查交付完成；来源已核对，正式业务未执行。"))
			resp.Close()
			return resp, nil
		}
		if strings.Contains(prompt, "你是调查子 Agent。") {
			call := childCalls.Add(1)
			record(fmt.Sprintf("调查子 Agent 第 %d 次调用", call))
			switch call {
			case 1:
				if err := assertSleeping(); err != nil {
					return nil, err
				}
				return protocolResponse(cfg, req, native, "directly_call_tool", map[string]any{"directly_call_tool_name": "read_file", "directly_call_tool_params": map[string]any{"file": fixture}, "directly_call_reason": "CHILD_PRIVATE_REASON_SENTINEL"})
			case 2:
				if !strings.Contains(prompt, observation) {
					return nil, fmt.Errorf("investigator real read missing")
				}
				return protocolResponse(cfg, req, native, "save_evidence", map[string]any{"evidence_id": "phase.discovery", "evidence_content": "共享发现：" + observation})
			case 3:
				select {
				case <-discoveryRead:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				if err := assertSleeping(); err != nil {
					return nil, err
				}
				return protocolResponse(cfg, req, native, "save_evidence", map[string]any{"evidence_id": "phase.discovery", "evidence_content": "共享发现：" + observation})
			case 4:
				before := mainCalls.Load()
				time.Sleep(30 * time.Millisecond)
				if before != mainCalls.Load() {
					return nil, fmt.Errorf("duplicate evidence woke planner")
				}
				return protocolResponse(cfg, req, native, "directly_answer", map[string]any{"answer_payload": "调查交付：来源已核对；证据 phase.discovery，正式业务尚未执行。"})
			default:
				return protocolResponse(cfg, req, native, "finish", map[string]any{})
			}
		}
		if req.GetCallerLabel() != "react-loop:coordinator" {
			return nil, fmt.Errorf("unexpected auxiliary or business-worker call: %s", req.GetCallerLabel())
		}
		mainCalls.Add(1)
		if index >= len(steps) {
			return nil, fmt.Errorf("smoke did not converge")
		}
		current := steps[index]
		record(fmt.Sprintf("协调员 %02d：%s", index, current.name))
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		projection := aiprojection.Project(aiprojection.ProjectionInput{Prompt: prompt, ActionTools: wire.Tools})
		if strings.Contains(prompt, "阶段：PLAN") {
			require.NotContains(t, prompt, "plan_version")
			require.Contains(t, prompt, query)
			require.Contains(t, prompt, "补充约束：仅规划")
			require.Contains(t, prompt, "环境观测：source.txt 已存在")
			if source != "investigation" {
				require.Contains(t, prompt, "已有来源证据，加载计划不得丢失。")
			}
			require.NotContains(t, prompt, "CHILD_PRIVATE_REASON_SENTINEL")
			var declarations any = wire.Tools
			if !native {
				match := regexp.MustCompile("(?s)```jsonschema\\n(.*?)\\n```").FindStringSubmatch(prompt)
				if len(match) != 2 {
					return nil, fmt.Errorf("text stream schema missing")
				}
				declarations = json.RawMessage(match[1])
			}
			tools, _ := json.Marshal(declarations)
			if native {
				names := map[string]bool{}
				for _, tool := range wire.Tools {
					names[tool.Function.Name] = true
				}
				for _, name := range []string{"start_tasks", "wait_tasks", "review_task", "retry_task", "cancel_tasks", "write_report"} {
					require.False(t, names[name])
				}
				require.Equal(t, explore, names["dispatch_sub_react_agents"])
			}
			var sections [3][32]byte
			for i, message := range []int{0, 1, 3} {
				raw, _ := json.Marshal(projection.Messages[message])
				sections[i] = sha256.Sum256(raw)
			}
			if index == 0 {
				stableSections = sections
				toolHash = sha256.Sum256(tools)
			} else {
				require.Equal(t, toolHash, sha256.Sum256(tools), "PLAN tools must be byte stable")
				require.Equal(t, stableSections, sections, "edits must not rewrite static/frozen/role partitions")
			}
			if root, err := currentDefinition(prompt); err == nil {
				if originalIDs == nil {
					originalIDs = map[string]string{"read": definitionNode(root, "read").TaskID, "verify": definitionNode(root, "verify").TaskID}
				}
				if n := definitionNode(root, "verify"); n != nil {
					require.Equal(t, originalIDs["verify"], n.TaskID)
				}
				if n := definitionNode(root, "read_checked"); n != nil {
					require.Equal(t, originalIDs["read"], n.TaskID)
				}
			}
			if index >= len(steps)-4 {
				require.Contains(t, prompt, finalDoc)
				require.Equal(t, 1, strings.Count(fmt.Sprint(projection.Messages[2].Content), finalDoc), "one current document in SemiDynamic1")
				var prefix [4][32]byte
				bytes := 0
				for i := range prefix {
					raw, _ := json.Marshal(projection.Messages[i])
					prefix[i] = sha256.Sum256(raw)
					bytes += len(raw)
				}
				if stableComparisons > 0 {
					require.Equal(t, cachePrefix, prefix, "ordinary evidence probes keep stable messages")
					raw, _ := json.Marshal(projection.Messages)
					toolBytes := 0
					if native {
						toolBytes = len(tools)
					}
					reuse := float64(bytes+toolBytes) / float64(len(raw)+toolBytes)
					if reuse < minimumReuse {
						minimumReuse = reuse
					}
				}
				cachePrefix = prefix
				stableComparisons++
			}
			if reviewDir != "" && (current.name == "submit_plan" || index == len(steps)-5 || index == 3 || (index == 0 && source != "investigation")) {
				require.NoError(t, os.WriteFile(filepath.Join(reviewDir, fmt.Sprintf("%02d-prompt.txt", index)), []byte(prompt), 0600))
				data, _ := json.MarshalIndent(declarations, "", "  ")
				require.NoError(t, os.WriteFile(filepath.Join(reviewDir, fmt.Sprintf("%02d-tools.json", index)), data, 0600))
				data, _ = json.MarshalIndent(projection.Messages, "", "  ")
				require.NoError(t, os.WriteFile(filepath.Join(reviewDir, fmt.Sprintf("%02d-messages.json", index)), data, 0600))
			}
			// Sample the actual provider-visible current view. The caller may be tier-wrapped.
			previousDocument = fmt.Sprint(projection.Messages[2].Content)
			previousDefinition = previousDocument
		}
		args, err := current.args(prompt)
		if err != nil {
			return nil, err
		}
		index++
		return protocolResponse(cfg, req, native, current.name, args)
	}
	finished := make(chan error, 1)
	event := func(op aicommon.AIEngineOperator, e *schema.AiOutputEvent) {
		var payload map[string]any
		_ = json.Unmarshal(e.Content, &payload)
		if payload["code"] == "plan.waiting_for_exploration" {
			waits.Add(1)
			select {
			case waiting <- struct{}{}:
			default:
			}
		}
		if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
			approvals.Add(1)
			mu.Lock()
			reviewPayload = payload
			coordinatorID = e.CoordinatorId
			mu.Unlock()
			plans := payload["plans"].(map[string]any)
			if plans["document"] != finalDoc {
				record("审核文档与最终文档不一致")
			}
			data, _ := json.Marshal(map[string]any{"suggestion": "continue", "plans": map[string]any{"root_task": plans["root_task"], "document": finalDoc + "用户确认的最终范围。\n"}})
			if err := op.SendInputEvent(&ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: payload["id"].(string), InteractiveJSONInput: string(data)}); err != nil {
				record(err.Error())
			}
		}
	}
	opts := []aicommon.ConfigOption{aicommon.WithEnableFunctionCallMode(native), aicommon.WithEnableSubagentsInPlan(explore), aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithGenerateReport(false), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithForceManualPlanReview(true), aicommon.WithAITransactionAutoRetry(1), aicommon.WithAIAutoRetry(1)}
	if source == "preset" {
		opts = append(opts, coordinator.WithPresetPlan(string(rawPlan), initialDoc))
	}
	if source == "mocker" {
		p, err := coordinator.ParsePlan(string(rawPlan), initialDoc, nil)
		require.NoError(t, err)
		var root coordinator.PlanNode
		require.NoError(t, json.Unmarshal(p.Tree, &root))
		opts = append(opts, coordinator.WithPlanMocker(func(*coordinator.Session) *coordinator.PlanResponse {
			mockerCalls.Add(1)
			return &coordinator.PlanResponse{RootTask: &root, Document: initialDoc}
		}))
	}
	setup := func(op aicommon.AIEngineOperator, input string) {
		r := op.(*aireact.ReAct)
		cfg := r.GetConfig().(*aicommon.Config)
		cfg.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(cfg.GetRuntimeId(), db)
		task := aicommon.NewStatefulTaskBase("plan-phase-fixture", input, ctx, cfg.GetEmitter(), true)
		r.SetCurrentTask(task)
		cfg.Timeline.EnsureTaskUserInput(task.GetId(), input, cfg.AcquireId)
		cfg.Timeline.SetTimelineBucketByteSize(-1)
		_, err := cfg.AppendUserInputHistory("补充约束：仅规划，不执行命令，不改业务文件。", time.Now())
		require.NoError(t, err)
		cfg.Timeline.PushText(cfg.AcquireId(), "环境观测：source.txt 已存在，当前工作区可读。")
		if source != "investigation" {
			cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "phase.preset", Content: "已有来源证据，加载计划不得丢失。"}})
		}
		cfg.Timeline.FreezeAll()
	}
	engine, err := aiengine.NewAIEngine(aiengine.WithPlanEngine("coordinator"), aiengine.WithMaxIteration(40), aiengine.WithTimeout(30), aiengine.WithReviewPolicy("yolo"), aiengine.WithAICallback(model), aiengine.WithOnEvent(event), aiengine.WithWorkdir(workdir), aiengine.WithExtOptions(opts...))
	require.NoError(t, err)
	defer engine.Close()
	setup(engine.GetOperator(), query)
	engine.GetOperator().(*aireact.ReAct).AsyncPlanOnly(ctx, query, func(err error) { finished <- err })
	select {
	case err = <-finished:
	case <-ctx.Done():
		err = ctx.Err()
	}
	mu.Lock()
	t.Log(strings.Join(trace, "\n"))
	mu.Unlock()
	require.NoError(t, err)
	require.Equal(t, len(steps), index)
	require.EqualValues(t, 1, approvals.Load(), "premature exploration submit never opens review")
	if explore {
		require.EqualValues(t, 2, waits.Load())
		require.GreaterOrEqual(t, childCalls.Load(), int64(4))
	} else {
		require.Zero(t, childCalls.Load())
	}
	if source == "mocker" {
		require.EqualValues(t, 1, mockerCalls.Load())
	}
	require.Contains(t, previousDocument, finalDoc)
	require.Contains(t, previousDefinition, "read_checked")
	mu.Lock()
	id := coordinatorID
	approvedReview := reviewPayload
	mu.Unlock()
	require.NotEmpty(t, id)
	row, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, id)
	require.NoError(t, err)
	var progress coordinator.Progress
	require.NoError(t, json.Unmarshal([]byte(row.TaskProgress), &progress))
	state := progress.CoordinatorState
	require.Equal(t, coordinator.PhaseExec, state.Phase)
	require.False(t, state.ReviewPending)
	require.Equal(t, finalDoc+"用户确认的最终范围。\n", state.Plan.Document)
	require.Len(t, state.Plan.Tasks, 2)
	for _, a := range state.Attempts {
		require.Zero(t, a.ID, "no business worker starts during first-stage smoke")
		require.Equal(t, coordinator.Pending, a.State)
	}
	require.Equal(t, []string{state.Plan.Tasks[0].ID}, state.Plan.Tasks[1].DependsOn)
	require.Equal(t, finalDoc, approvedReview["plans"].(map[string]any)["document"])
	t.Logf("source=%s native=%t exploration=%t main=%d child=%d automatic-waits=%d approvals=1 stable-comparisons=%d minimum-reusable-message/tool-bytes=%.2f%%", source, native, explore, mainCalls.Load(), childCalls.Load(), waits.Load(), stableComparisons, minimumReuse*100)
	if reviewDir != "" {
		summary, _ := json.MarshalIndent(map[string]any{"source": source, "native": native, "exploration": explore, "main_requests": mainCalls.Load(), "child_requests": childCalls.Load(), "auxiliary_requests": auxiliaryCalls.Load(), "automatic_waits": waits.Load(), "approvals": approvals.Load(), "business_worker_starts": 0, "minimum_reusable_message_and_tool_bytes": minimumReuse, "provider_cache_hit_rate": nil, "state": state, "trace": trace}, "", "  ")
		require.NoError(t, os.WriteFile(filepath.Join(reviewDir, "summary.json"), summary, 0600))
	}
}
