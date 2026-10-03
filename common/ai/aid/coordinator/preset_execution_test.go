package coordinator_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

func TestCoordinatorPresetExecutesDependentWorkers(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("function_call_%v", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var session *coordinator.Session
			var mu sync.Mutex
			steps := map[string]int{}
			reviews := 0
			calls := 0
			prompts := map[string]string{}
			s, err := coordinator.NewSession(ctx, "只读核对两份来源，确认一次后自动执行。",
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithWorkdir(t.TempDir()), aicommon.WithGenerateReport(false), aicommon.WithAgreeYOLO(),
				coordinator.WithPresetPlan(`{"main_task":"Sources","main_task_goal":"Verify sources","tasks":[{"subtask_name":"First","subtask_goal":"Read source A","subtask_identifier":"first","depends_on":[]},{"subtask_name":"Second","subtask_goal":"Verify against source A","subtask_identifier":"second","depends_on":["first"]}]}`, "# 来源核对\n先确认第一份来源，再复核第二份来源。"),
				aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
					if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
						mu.Lock()
						reviews++
						mu.Unlock()
					}
				}),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					mu.Lock()
					defer mu.Unlock()
					calls++
					if calls > 60 {
						return nil, fmt.Errorf("preset execution did not converge")
					}
					prompt := req.GetPrompt()
					if calls == 1 {
						prompts["00-preset.txt"] = prompt
						if native {
							wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
							seen := map[string]bool{}
							for _, tool := range wire.Tools {
								for _, name := range coordinator.ActionNames {
									if tool.Function.Name == name {
										seen[name] = true
										require.Contains(t, tool.Function.Description, "原生调用通过函数名")
										require.NotContains(t, tool.Function.Description, "文本流模式")
									}
								}
							}
							require.Len(t, seen, 3)
						} else {
							require.Contains(t, prompt, "文本流模式通过 @action")
							require.Contains(t, prompt, "嵌套 PLAN DAG")
							require.NotContains(t, prompt, "原生调用通过函数名")
						}
					}
					if native {
						require.NotContains(t, prompt, "本轮使用文本流 JSON action")
					} else {
						require.Contains(t, prompt, "文本流 JSON")
						require.NotContains(t, prompt, "本轮使用原生 function call：")
					}
					if strings.Contains(prompt, "执行已批准的冻结任务书。") {
						snapshot := session.Snapshot()
						index := snapshot.Attempts[req.GetTaskIndex()].Task.Index
						if index == "" && !native {
							// The text-stream request does not carry native task metadata;
							// this fixture dispatches one worker at a time.
							for _, attempt := range snapshot.Attempts {
								if attempt.State == coordinator.Running {
									index = attempt.Task.Index
								}
							}
						}
						if index == "" {
							return nil, fmt.Errorf("unknown worker task %q", req.GetTaskIndex())
						}
						step := steps[index]
						steps[index]++
						if index == "2" {
							if !strings.Contains(prompt, "preset.source.1") {
								return nil, fmt.Errorf("dependent worker did not receive upstream session evidence")
							}
							prompts["05-dependent-worker.txt"] = prompt
						}
						switch step {
						case 0:
							return protocolResponse(c, req, native, "save_evidence", map[string]any{"evidence_id": "preset.source." + index, "evidence_content": "来源 " + index + " 已在本地核对，观测与分配的任务书一致，供后继任务复核。"})
						case 1:
							return protocolResponse(c, req, native, "submit_task_result", map[string]any{"summary": "来源 " + index + " 核对完成", "evidence_ids": []string{"preset.source." + index}})
						default:
							return protocolResponse(c, req, native, "finish", map[string]any{})
						}
					}
					snapshot := session.Snapshot()
					if snapshot.Phase == coordinator.PhasePlan {
						return protocolResponse(c, req, native, "submit_plan", map[string]any{})
					}
					for _, task := range snapshot.Plan.Tasks {
						a := snapshot.Attempts[task.ID]
						if a.State == coordinator.AwaitingReview {
							prompts["04-awaiting-review.txt"] = prompt
							return protocolResponse(c, req, native, "review_task", map[string]any{"task_id": task.ID, "attempt_id": a.ID, "decision": "accept", "reason": "结果与实际保存的 Evidence 一致"})
						}
					}
					for _, a := range snapshot.Attempts {
						if a.State == coordinator.Running {
							return protocolResponse(c, req, native, "directly_answer", map[string]any{"answer_payload": "已检查新发现，继续执行。"})
						}
					}
					for _, a := range snapshot.Attempts {
						if a.State == coordinator.Pending {
							return protocolResponse(c, req, native, "wait_messages", map[string]any{})
						}
					}
					return reportResponse(c, req, native, snapshot.Report.Path != "")
				}))
			require.NoError(t, err)
			session = s
			defer s.Close()
			require.NoError(t, s.Run())
			snapshot := s.Snapshot()
			require.True(t, snapshot.Finished)
			require.Len(t, snapshot.Attempts, 2)
			for _, a := range snapshot.Attempts {
				require.Equal(t, coordinator.Accepted, a.State)
			}
			require.Equal(t, 1, reviews, "only the plan approval emits a user review")
			require.Contains(t, s.GetSessionEvidenceRendered(), "preset.source.1")
			require.Contains(t, s.GetSessionEvidenceRendered(), "preset.source.2")
			if dir := os.Getenv("COORDINATOR_CONTEXT_REVIEW_DIR"); dir != "" {
				dir = filepath.Join(dir, fmt.Sprintf("function-call-%v", native))
				require.NoError(t, os.MkdirAll(dir, 0700))
				for name, prompt := range prompts {
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(prompt), 0600))
				}
			}

		})
	}
}
