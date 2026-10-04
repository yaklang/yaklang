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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Only decisions and user replies are scripted. Engine, scheduler, independent
// pe_task contexts, tools, evidence, inbox, approval and report storage are real.
func TestCoordinatorExecutionPhaseMatrix(t *testing.T) {
	for _, native := range []bool{true, false} {
		for _, manual := range []bool{true, false} {
			t.Run(fmt.Sprintf("function_call=%t/manual=%t", native, manual), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				workdir := t.TempDir()
				source := filepath.Join(workdir, "source.txt")
				require.NoError(t, os.WriteFile(source, []byte("execution-source-sentinel: validated local source"), 0600))
				var mu, eventMu sync.Mutex
				steps := map[string]int{}
				starts := map[string]uint64{}
				mainCalls, workerCalls, modelReviews, reportStep := 0, 0, 0, 0
				planReviews, taskReviews, reportEvents, endEvents, pushes, pops := 0, 0, 0, 0, 0, 0
				var reportPath string
				var trace, mainPrompts []string
				var permanent [2][32]byte
				toolsByRole := map[string][32]byte{}
				cacheSamples := []map[string]any{}
				var eventErr error
				briefPattern := regexp.MustCompile(`\[CURRENT_EXECUTION\]\n([^\n]+)`)
				statusPattern := regexp.MustCompile(`\[([^\]]+)\]: ([a-z_]+); attempt=(\d+)`)
				model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					mu.Lock()
					defer mu.Unlock()
					prompt := req.GetPrompt()
					if b := briefPattern.FindStringSubmatch(prompt); len(b) > 0 {
						var brief struct {
							TaskID       string            `json:"task_id"`
							Name         string            `json:"name"`
							AttemptID    uint64            `json:"attempt_id"`
							Predecessors []json.RawMessage `json:"accepted_predecessors"`
						}
						if err := json.Unmarshal([]byte(b[1]), &brief); err != nil {
							return nil, err
						}
						workerCalls++
						step := steps[brief.Name]
						steps[brief.Name]++
						if step == 0 {
							if _, exists := starts[brief.Name]; exists {
								return nil, fmt.Errorf("duplicate dispatch %s", brief.Name)
							}
							starts[brief.Name] = brief.AttemptID
							trace = append(trace, "worker开始 "+brief.Name)
							if brief.Name == "C" && len(brief.Predecessors) != 1 {
								return nil, fmt.Errorf("C lacks accepted A")
							}
							if brief.Name == "D" && len(brief.Predecessors) != 2 {
								return nil, fmt.Errorf("D lacks accepted B/C")
							}
							return protocolResponse(cfg, req, native, "directly_call_tool", map[string]any{"directly_call_tool_name": "read_file", "directly_call_tool_params": map[string]any{"file": source}, "directly_call_reason": "读取实际来源并独立验证。"})
						}
						if !strings.Contains(prompt, "execution-source-sentinel") {
							return nil, fmt.Errorf("read_file output missing for %s", brief.Name)
						}
						switch step {
						case 1:
							return protocolResponse(cfg, req, native, "save_evidence", map[string]any{"evidence_id": "execution." + brief.Name, "evidence_content": "来源 source.txt 已由任务 " + brief.Name + " 实际读取，execution-source-sentinel 与任务要求一致。"})
						case 2:
							return protocolResponse(cfg, req, native, "submit_task_result", map[string]any{"summary": brief.Name + " 已读取并验证真实来源", "evidence_ids": []string{"execution." + brief.Name}})
						default:
							return protocolResponse(cfg, req, native, "finish", map[string]any{})
						}
					}
					mainCalls++
					mainPrompts = append(mainPrompts, prompt)
					wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					projected := aiprojection.Project(aiprojection.ProjectionInput{Prompt: prompt, ActionTools: wire.Tools})
					if root := os.Getenv("COORDINATOR_EXEC_REVIEW_DIR"); root != "" {
						dir := filepath.Join(root, fmt.Sprintf("fc-%t-manual-%t", native, manual))
						if err := os.MkdirAll(dir, 0700); err != nil {
							return nil, err
						}
						data, _ := json.MarshalIndent(projected.Messages, "", "  ")
						if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("provider-messages-%02d.json", mainCalls)), data, 0600); err != nil {
							return nil, err
						}
					}
					if !projected.Metadata.CacheProjected || len(projected.Messages) < 5 {
						return nil, fmt.Errorf("cache regions collapsed")
					}
					var fixed [2][32]byte
					fixedBytes := 0
					for i := range fixed {
						data, _ := json.Marshal(projected.Messages[i])
						fixed[i] = sha256.Sum256(data)
						fixedBytes += len(data)
					}
					if mainCalls == 1 {
						permanent = fixed
					} else if fixed != permanent {
						return nil, fmt.Errorf("task notifications changed High Static/Frozen: request=%d high_static=%t frozen=%t", mainCalls, fixed[0] != permanent[0], fixed[1] != permanent[1])
					}
					role := "PLAN"
					if strings.Contains(prompt, "阶段：EXEC") {
						role = "EXEC"
						if strings.Contains(prompt, "当前任务图和关键消息已经收尾") {
							role += "/report"
						}
					}
					toolData, _ := json.Marshal(projected.Tools)
					toolHash := sha256.Sum256(toolData)
					if previous, exists := toolsByRole[role]; exists && role != "PLAN" && previous != toolHash {
						return nil, fmt.Errorf("notification changed tool contracts within %s", role)
					}
					toolsByRole[role] = toolHash
					messageData, _ := json.Marshal(projected.Messages)
					cacheSamples = append(cacheSamples, map[string]any{"request": mainCalls, "role": role, "high_static_hash": fmt.Sprintf("%x", fixed[0]), "frozen_hash": fmt.Sprintf("%x", fixed[1]), "tool_hash": fmt.Sprintf("%x", toolHash), "permanent_message_byte_ratio": float64(fixedBytes) / float64(len(messageData)), "provider_cache_hit_rate": nil})
					if mainCalls > 30 {
						return nil, fmt.Errorf("main model did not converge")
					}
					if mainCalls == 1 {
						return protocolResponse(cfg, req, native, "directly_call_tool", map[string]any{"directly_call_tool_name": "read_file", "directly_call_tool_params": map[string]any{"file": source}, "directly_call_reason": "规划前确认来源。"})
					}
					if strings.Contains(prompt, "已有计划：false") {
						tasks := []any{}
						for _, name := range []string{"A", "B", "C", "D"} {
							deps := []string{}
							if name == "C" {
								deps = []string{"a"}
							}
							if name == "D" {
								deps = []string{"b", "c"}
							}
							tasks = append(tasks, map[string]any{"name": name, "goal": "只读验证 source.txt，保存实际 Evidence 和结果。", "identifier": strings.ToLower(name), "depends_on": deps})
						}
						return protocolResponse(cfg, req, native, "create_plan", map[string]any{"plan": map[string]any{"name": "执行阶段实验", "goal": "验证自动DAG和审核交付", "tasks": tasks}, "plan_document": "# 执行阶段实验\nA/B独立，C等待A审核，D等待B/C审核。只读验证本地来源，不访问外部网络。"})
					}
					status := prompt[strings.LastIndex(prompt, "# PLAN STATUS"):]
					if strings.Contains(status, "阶段：PLAN") {
						return protocolResponse(cfg, req, native, "submit_plan", map[string]any{})
					}
					matches := statusPattern.FindAllStringSubmatch(status, -1)
					accepted := 0
					for _, m := range matches {
						if m[2] == "accepted" {
							accepted++
						}
						if m[2] == "awaiting_review" && !manual {
							modelReviews++
							trace = append(trace, "YOLO质量审核 "+m[1])
							return protocolResponse(cfg, req, native, "review_task", map[string]any{"task_id": m[1], "attempt_id": json.Number(m[3]), "decision": "accept", "reason": "实际 read_file 输出、共享 execution Evidence 和提交结果一致。"})
						}
						if m[2] == "failed" || m[2] == "rejected" {
							return nil, fmt.Errorf("unexpected failed task: %s", status)
						}
					}
					if accepted < 4 || !strings.Contains(prompt, "当前任务图和关键消息已经收尾") {
						return protocolResponse(cfg, req, native, "directly_answer", map[string]any{"answer_payload": "已检查新增证据，继续由运行时等待任务与用户决定。"})
					}
					if !strings.Contains(prompt, "execution.A") || !strings.Contains(prompt, "execution.D") {
						return nil, fmt.Errorf("shared Evidence was not handed over")
					}
					reportStep++
					switch reportStep {
					case 1:
						return protocolResponse(cfg, req, native, "create_report", map[string]any{"title": "DAG执行检查", "document": "# DAG执行检查\nA/B/C/D已审核；证据：execution.A、execution.B、execution.C、execution.D。\n"})
					case 2:
						if !strings.Contains(prompt, "# CURRENT REPORT") {
							return nil, fmt.Errorf("current report missing from context")
						}
						return protocolResponse(cfg, req, native, "modify_report", map[string]any{"document_patch": "--- coordinator-report.md\n+++ coordinator-report.md\n@@ -1,2 +1,3 @@\n # DAG执行检查\n A/B/C/D已审核；证据：execution.A、execution.B、execution.C、execution.D。\n+运行时自动派发；未发生重复执行；仅访问本地 source.txt。\n"})
					default:
						return protocolResponse(cfg, req, native, "submit_report", map[string]any{"summary": "DAG和真实证据验证完成，交付最新报告。"})
					}
				}
				event := func(op aicommon.AIEngineOperator, e *schema.AiOutputEvent) {
					eventMu.Lock()
					defer eventMu.Unlock()
					var data map[string]any
					_ = json.Unmarshal(e.Content, &data)
					if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE || e.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE || e.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
						if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
							planReviews++
						}
						if e.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
							taskReviews++
						}
						id, _ := data["id"].(string)
						if err := op.SendInputEvent(&ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: id, InteractiveJSONInput: `{"suggestion":"continue"}`}); err != nil {
							eventErr = err
						}
					}
					if e.Type == schema.EVENT_TYPE_REPORT_FINISH {
						reportEvents++
						reportPath, _ = data["report_path"].(string)
					}
					if e.Type == schema.EVENT_TYPE_END_PLAN_AND_EXECUTION {
						endEvents++
					}
					if e.NodeId == "system" {
						if data["type"] == "push_task" {
							pushes++
						}
						if data["type"] == "pop_task" {
							pops++
						}
					}
				}
				policy := "yolo"
				if manual {
					policy = "manual"
				}
				err := aiengine.InvokeReAct("创建依赖任务，读取本地来源，保存 Evidence、验收并交付报告。", aiengine.WithContext(ctx), aiengine.WithPlanEngine("coordinator"), aiengine.WithMaxIteration(40), aiengine.WithTimeout(30), aiengine.WithReviewPolicy(policy), aiengine.WithAICallback(model), aiengine.WithOnEvent(event), aiengine.WithWorkdir(workdir), aiengine.WithExtOptions(aicommon.WithDisableSessionTitleGeneration(true), aicommon.WithAICallback(model), aicommon.WithEnableFunctionCallMode(native), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithForceManualPlanReview(true), aicommon.WithPlanExecTaskConcurrency(2), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1)))
				require.NoError(t, err)
				mu.Lock()
				defer mu.Unlock()
				eventMu.Lock()
				defer eventMu.Unlock()
				require.NoError(t, eventErr)
				require.Equal(t, 1, planReviews)
				require.Equal(t, 1, reportEvents)
				require.Equal(t, 1, endEvents)
				require.Equal(t, 4, pushes)
				require.Equal(t, 4, pops)
				require.Len(t, starts, 4)
				require.Equal(t, 16, workerCalls)
				if manual {
					require.Equal(t, 4, taskReviews)
					require.Zero(t, modelReviews)
				} else {
					require.Zero(t, taskReviews)
					require.Equal(t, 4, modelReviews)
				}
				body, err := os.ReadFile(reportPath)
				require.NoError(t, err)
				require.Contains(t, string(body), "未发生重复执行")
				t.Logf("真实 Yak/aim：主模型=%d worker模型=%d 启动=%v 人工任务审核=%d YOLO审核=%d报告交付=%d", mainCalls, workerCalls, starts, taskReviews, modelReviews, reportEvents)
				if root := os.Getenv("COORDINATOR_EXEC_REVIEW_DIR"); root != "" {
					dir := filepath.Join(root, fmt.Sprintf("fc-%t-manual-%t", native, manual))
					require.NoError(t, os.MkdirAll(dir, 0700))
					for i, p := range mainPrompts {
						require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("prompt-%02d.txt", i+1)), []byte(p), 0600))
					}
					stats, _ := json.MarshalIndent(map[string]any{"main_calls": mainCalls, "worker_calls": workerCalls, "starts": starts, "manual_reviews": taskReviews, "model_reviews": modelReviews, "trace": trace, "cache_samples": cacheSamples, "report_deliveries": reportEvents, "end_events": endEvents}, "", "  ")
					require.NoError(t, os.WriteFile(filepath.Join(dir, "run.json"), stats, 0600))
					require.NoError(t, os.WriteFile(filepath.Join(dir, "report.md"), body, 0600))
				}
			})
		}
	}
}
