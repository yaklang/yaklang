package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestExecutionManualInaccurateFeedbackRetriesWithoutPlanReapproval(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			input := make(chan *ypb.AIInputEvent, 8)
			var s *coordinator.Session
			var mu sync.Mutex
			steps := map[uint64]int{}
			mainCalls := 0
			var planReviews, taskReviews atomic.Int64
			briefPattern := regexp.MustCompile(`\[CURRENT_EXECUTION\]\n([^\n]+)`)
			model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				mu.Lock()
				defer mu.Unlock()
				prompt := req.GetPrompt()
				if match := briefPattern.FindStringSubmatch(prompt); len(match) > 0 {
					var brief struct {
						Attempt uint64 `json:"attempt_id"`
						Goal    string `json:"goal"`
					}
					if err := json.Unmarshal([]byte(match[1]), &brief); err != nil {
						return nil, err
					}
					if len(steps) > 0 && steps[brief.Attempt] == 0 && !strings.Contains(brief.Goal, "核对原始来源") {
						return nil, fmt.Errorf("retry did not receive user's requirement")
					}
					steps[brief.Attempt]++
					if steps[brief.Attempt] == 1 {
						return protocolResponse(cfg, req, native, "submit_task_result", map[string]any{"summary": "当前尝试已核对任务书"})
					}
					return protocolResponse(cfg, req, native, "finish", map[string]any{})
				}
				mainCalls++
				if mainCalls > 20 {
					return nil, fmt.Errorf("coordinator did not converge")
				}
				if mainCalls == 1 {
					return protocolResponse(cfg, req, native, "create_plan", map[string]any{"plan": map[string]any{"name": "反馈复核", "goal": "保持人工任务审核", "tasks": []any{map[string]any{"name": "复核", "goal": "检查结果", "identifier": "check"}}}, "plan_document": "# 反馈复核\n按当前任务书执行，用户可要求重查。"})
				}
				if mainCalls == 2 {
					return protocolResponse(cfg, req, native, "submit_plan", map[string]any{})
				}
				return reportResponse(cfg, req, native, s.Snapshot().Report.Path != "")
			}
			var err error
			s, err = coordinator.NewSession(ctx, "生成计划，经确认后执行。", aicommon.WithWorkdir(t.TempDir()), aicommon.WithEventInputChan(input), aicommon.WithEnableFunctionCallMode(native), aicommon.WithForceManualPlanReview(true), aicommon.WithAgreeManual(), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithAITransactionAutoRetry(1), aicommon.WithAICallback(model), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
				if e.Type != schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE && e.Type != schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
					return
				}
				var data map[string]any
				_ = json.Unmarshal(e.Content, &data)
				id, _ := data["id"].(string)
				reply := `{"suggestion":"continue"}`
				if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
					planReviews.Add(1)
				}
				if e.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE && taskReviews.Add(1) == 1 {
					reply = `{"suggestion":"inaccurate","extra_prompt":"核对原始来源，不能沿用浅层结论"}`
				}
				input <- &ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: id, InteractiveJSONInput: reply}
			}))
			require.NoError(t, err)
			defer s.Close()
			require.NoError(t, s.Run())
			require.EqualValues(t, 1, planReviews.Load())
			require.EqualValues(t, 2, taskReviews.Load())
			require.Len(t, steps, 2)
			state := s.Snapshot()
			for id, attempt := range state.Attempts {
				require.Equal(t, coordinator.Accepted, attempt.State)
				require.Contains(t, attempt.Task.Goal, "核对原始来源")
				require.Len(t, state.History[id], 2)
				require.Equal(t, coordinator.Rejected, state.History[id][0].State)
			}
			require.True(t, state.Finished)
		})
	}
}

func TestExecutionAutomaticAIPoliciesUseCoordinatorQualityReview(t *testing.T) {
	for _, native := range []bool{true, false} {
		for _, policy := range []aicommon.AgreePolicyType{aicommon.AgreePolicyAI, aicommon.AgreePolicyAIAuto} {
			t.Run(fmt.Sprintf("native=%t/policy=%s", native, policy), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				input := make(chan *ypb.AIInputEvent, 4)
				var s *coordinator.Session
				var mu sync.Mutex
				mainCalls, workerCalls, modelReviews := 0, 0, 0
				var taskReviews atomic.Int64
				model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					mu.Lock()
					defer mu.Unlock()
					if strings.Contains(req.GetPrompt(), "[CURRENT_EXECUTION]") {
						workerCalls++
						if workerCalls == 1 {
							return protocolResponse(cfg, req, native, "submit_task_result", map[string]any{"summary": "已核对冻结任务书，供协调员验收"})
						}
						return protocolResponse(cfg, req, native, "finish", map[string]any{})
					}
					mainCalls++
					if mainCalls > 16 {
						return nil, fmt.Errorf("automatic review did not converge")
					}
					if mainCalls == 1 {
						return protocolResponse(cfg, req, native, "create_plan", map[string]any{"plan": map[string]any{"name": "策略兼容", "goal": "自动质量判断", "tasks": []any{map[string]any{"name": "核对", "goal": "核对本次任务书", "identifier": "check"}}}, "plan_document": "# 自动审阅策略兼容"})
					}
					if mainCalls == 2 {
						return protocolResponse(cfg, req, native, "submit_plan", map[string]any{})
					}
					for _, a := range s.Snapshot().Attempts {
						if a.State == coordinator.AwaitingReview {
							modelReviews++
							return protocolResponse(cfg, req, native, "review_task", map[string]any{"task_id": a.Task.ID, "attempt_id": a.ID, "decision": "accept", "reason": "已核对 Timeline 中的实际任务结果及冻结目标。"})
						}
					}
					return reportResponse(cfg, req, native, s.Snapshot().Report.Path != "")
				}
				var err error
				s, err = coordinator.NewSession(ctx, "计划确认一次，按原有自动策略完成质量检查。", aicommon.WithWorkdir(t.TempDir()), aicommon.WithEventInputChan(input), aicommon.WithEnableFunctionCallMode(native), aicommon.WithAgreePolicy(policy), aicommon.WithForceManualPlanReview(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1), aicommon.WithAICallback(model), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
					if e.Type != schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE && e.Type != schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
						return
					}
					if e.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
						taskReviews.Add(1)
					}
					var data map[string]any
					_ = json.Unmarshal(e.Content, &data)
					id, _ := data["id"].(string)
					input <- &ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: id, InteractiveJSONInput: `{"suggestion":"continue"}`}
				}))
				require.NoError(t, err)
				defer s.Close()
				require.NoError(t, s.Run())
				require.Zero(t, taskReviews.Load(), "AI policies must not silently force human task review")
				require.Equal(t, 1, modelReviews, "automatic preferences still require explicit quality judgment")
				require.True(t, s.Snapshot().Finished)
			})
		}
	}
}
