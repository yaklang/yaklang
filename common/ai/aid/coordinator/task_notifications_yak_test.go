package coordinator_test

import (
	"context"
	_ "embed"
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
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed smoke/task_notifications.yak
var notificationYakSmoke string

// Both protocols execute this one Yak script. Only model decisions are scripted;
// approval, worker execution, evidence, notification and finish gates are real.
func TestCoordinatorYakAutomaticTaskNotifications(t *testing.T) {
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
				if !strings.Contains(prompt, "Draft version: 1") {
					return protocolResponse(cfg, req, native, "create_plan", map[string]any{"plan": map[string]any{"main_task": "通知实验", "main_task_goal": "验证主协调员自动等待和唤醒", "tasks": []any{map[string]any{"subtask_name": "检查来源", "subtask_goal": "保存证据并提交验证结果", "subtask_identifier": "source", "depends_on": []string{}}}}, "plan_document": "# 通知实验\n子任务运行期间报告发现，完成后自动验收。"})
				}
				if strings.Contains(prompt, "approved version: 0") {
					return protocolResponse(cfg, req, native, "submit_plan", map[string]any{"plan_version": 1})
				}
				match := pattern.FindStringSubmatch(prompt)
				if len(match) == 0 {
					return nil, fmt.Errorf("task status missing")
				}
				switch match[2] {
				case "pending":
					record("主协调员：start_tasks")
					return protocolResponse(cfg, req, native, "start_tasks", map[string]any{})
				case "running":
					if strings.Contains(prompt, "notification-discovery-sentinel") {
						record("主协调员：被发现通知唤醒，读取到新证据；任务仍 running")
						checked.Do(func() { close(discoveryChecked) })
						return protocolResponse(cfg, req, native, "directly_answer", map[string]any{"answer_payload": "已经收到子任务的新发现，继续等待完成。"})
					}
					record("主协调员：无其他工作，finish 转为系统自动等待")
					return protocolResponse(cfg, req, native, "finish", map[string]any{})
				case "awaiting_review":
					if !strings.Contains(prompt, "notification-result-sentinel") {
						return nil, fmt.Errorf("settlement woke planner before result publication")
					}
					record("主协调员：被结算通知唤醒，读取完整结果并验收")
					id, _ := strconv.ParseUint(match[3], 10, 64)
					return protocolResponse(cfg, req, native, "review_task", map[string]any{"task_id": match[1], "attempt_id": id, "decision": "accept", "reason": "notification.discovery 与实际完成结果一致"})
				case "accepted":
					record("主协调员：finish")
					return protocolResponse(cfg, req, native, "finish", map[string]any{})
				default:
					return nil, fmt.Errorf("unexpected task state: %s", match[2])
				}
			}
			event := func(op aicommon.AIEngineOperator, e *schema.AiOutputEvent) {
				var data map[string]any
				_ = json.Unmarshal(e.Content, &data)
				if data["code"] == "plan.waiting_for_tasks" {
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
			engine := yak.NewScriptEngine(1)
			workdir := t.TempDir()
			engine.RegisterEngineHooks(func(e *antlr4yak.Engine) error {
				e.SetVars(map[string]any{"NOTIFICATION_MODEL": model, "NOTIFICATION_EVENT": event, "SMOKE_WORKDIR": workdir, "SMOKE_OPTIONS": []aiengine.AIEngineConfigOption{aiengine.WithExtOptions(aicommon.WithEnableFunctionCallMode(native), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithGenerateReport(false), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithForceManualPlanReview(true))}})
				return nil
			})
			_, err := engine.ExecuteExWithContext(ctx, notificationYakSmoke, map[string]any{})
			mu.Lock()
			t.Log(strings.Join(trace, "\n"))
			mu.Unlock()
			require.NoError(t, err)
			require.EqualValues(t, 1, approvals.Load())
			require.EqualValues(t, 2, waitEvents.Load())
			require.EqualValues(t, 3, workerCalls.Load())
			require.LessOrEqual(t, mainCalls.Load(), int64(7), "no model polling or explicit wait action")
			t.Logf("Yak/aim notification smoke: planner=%d, worker=%d, automatic waits=%d, approvals=%d, explicit wait_tasks=0", mainCalls.Load(), workerCalls.Load(), waitEvents.Load(), approvals.Load())
		})
	}
}
