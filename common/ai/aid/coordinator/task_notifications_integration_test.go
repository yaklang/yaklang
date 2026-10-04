package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Both protocols exercise the AI engine. Only model decisions are scripted;
// approval, worker execution, evidence, notification and finish gates are real.
func TestCoordinatorAutomaticTaskNotifications(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			waiting := make(chan struct{}, 4)
			discoveryChecked := make(chan struct{})
			var checked sync.Once
			var mainCalls, workerCalls, waitEvents, approvals atomic.Int64
			var mu sync.Mutex
			var trace []string
			record := func(msg string) {
				mu.Lock()
				trace = append(trace, msg)
				mu.Unlock()
			}
			waitFor := func(ch <-chan struct{}) error {
				select {
				case <-ch:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			assertSleeping := func() error {
				if err := waitFor(waiting); err != nil {
					return err
				}
				before := mainCalls.Load()
				select {
				case <-time.After(80 * time.Millisecond):
				case <-ctx.Done():
					return ctx.Err()
				}
				if mainCalls.Load() != before {
					return fmt.Errorf("planner polled the model while sleeping: %d -> %d", before, mainCalls.Load())
				}
				record("系统挂起：80ms 内主模型调用数不变")
				return nil
			}
			pattern := regexp.MustCompile(`\[([^\]]+)\]: ([a-z_]+); attempt=(\d+)`)
			model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				prompt := req.GetPrompt()
				if strings.Contains(prompt, "执行已批准的冻结任务书。") {
					switch workerCalls.Add(1) {
					case 1:
						if err := assertSleeping(); err != nil {
							return nil, err
						}
						record("子任务：save_evidence 新发现")
						return protocolResponse(cfg, req, native, "save_evidence", map[string]any{"evidence_id": "notification.discovery", "evidence_content": "notification-discovery-sentinel：子任务执行中已确认来源，尚未结束。"})
					case 2:
						if err := waitFor(discoveryChecked); err != nil {
							return nil, err
						}
						if err := assertSleeping(); err != nil {
							return nil, err
						}
						record("子任务：提交结果")
						return protocolResponse(cfg, req, native, "submit_task_result", map[string]any{"summary": "notification-result-sentinel：来源已完整验证。", "evidence_ids": []string{"notification.discovery"}})
					default:
						record("子任务：finish，随后由 Controller 结算")
						return protocolResponse(cfg, req, native, "finish", map[string]any{})
					}
				}
				mainCalls.Add(1)
				if strings.Contains(prompt, "已有计划：false") {
					return protocolResponse(cfg, req, native, "create_plan", map[string]any{"plan": map[string]any{"main_task": "通知实验", "main_task_goal": "验证主协调员自动等待和唤醒", "tasks": []any{map[string]any{"subtask_name": "检查来源", "subtask_goal": "保存证据并提交验证结果", "subtask_identifier": "source", "depends_on": []string{}}}}, "plan_document": "# 通知实验\n子任务运行期间报告发现，完成后自动验收。"})
				}
				if strings.Contains(prompt, "阶段：PLAN") {
					return protocolResponse(cfg, req, native, "submit_plan", map[string]any{})
				}
				match := pattern.FindStringSubmatch(prompt)
				if len(match) == 0 {
					return nil, fmt.Errorf("task status missing")
				}
				switch match[2] {
				case "pending":
					record("运行时：批准后自动派发")
					return protocolResponse(cfg, req, native, "wait_messages", map[string]any{})
				case "running":
					if strings.Contains(prompt, "notification-discovery-sentinel") {
						record("主协调员：被发现通知唤醒，读取到新证据；任务仍 running")
						checked.Do(func() { close(discoveryChecked) })
						return protocolResponse(cfg, req, native, "directly_answer", map[string]any{"answer_payload": "已经收到子任务的新发现，继续等待完成。"})
					}
					record("主协调员：无其他工作，finish 转为系统自动等待")
					return protocolResponse(cfg, req, native, "wait_messages", map[string]any{})
				case "awaiting_review":
					if !strings.Contains(prompt, "notification-result-sentinel") {
						return nil, fmt.Errorf("settlement woke planner before result publication")
					}
					record("主协调员：被结算通知唤醒，读取完整结果并验收")
					id, _ := strconv.ParseUint(match[3], 10, 64)
					return protocolResponse(cfg, req, native, "review_task", map[string]any{"task_id": match[1], "attempt_id": id, "decision": "accept", "reason": "notification.discovery 与实际完成结果一致"})
				case "accepted":
					record("主协调员：finish")
					return reportResponse(cfg, req, native, strings.Contains(prompt, "# CURRENT REPORT"))
				default:
					return nil, fmt.Errorf("unexpected task state: %s", match[2])
				}
			}
			event := func(op aicommon.AIEngineOperator, e *schema.AiOutputEvent) {
				var data map[string]any
				_ = json.Unmarshal(e.Content, &data)
				if data["code"] == "plan.waiting_for_messages" {
					waitEvents.Add(1)
					select {
					case waiting <- struct{}{}:
					default:
					}
				}
				if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
					approvals.Add(1)
					id, _ := data["id"].(string)
					if err := op.SendInputEvent(&ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: id, InteractiveJSONInput: `{"suggestion":"continue"}`}); err != nil {
						record("审核发送失败：" + err.Error())
					}
				}
			}
			workdir := t.TempDir()
			err := aiengine.InvokeReAct("创建依赖任务，读取本地来源，保存 Evidence、验收并交付报告。", aiengine.WithContext(ctx), aiengine.WithPlanEngine("coordinator"), aiengine.WithMaxIteration(40), aiengine.WithTimeout(30), aiengine.WithReviewPolicy("yolo"), aiengine.WithAICallback(model), aiengine.WithOnEvent(event), aiengine.WithWorkdir(workdir), aiengine.WithExtOptions(aicommon.WithDisableSessionTitleGeneration(true), aicommon.WithAICallback(model), aicommon.WithEnableFunctionCallMode(native), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithGenerateReport(false), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithForceManualPlanReview(true)))
			mu.Lock()
			t.Log(strings.Join(trace, "\n"))
			mu.Unlock()
			require.NoError(t, err)
			require.EqualValues(t, 1, approvals.Load())
			require.EqualValues(t, 2, waitEvents.Load())
			require.EqualValues(t, 3, workerCalls.Load())
			require.LessOrEqual(t, mainCalls.Load(), int64(8), "no model polling or explicit wait action")
			t.Logf("Yak/aim notification smoke: planner=%d, worker=%d, automatic waits=%d, approvals=%d, explicit wait_messages=0", mainCalls.Load(), workerCalls.Load(), waitEvents.Load(), approvals.Load())
		})
	}
}
