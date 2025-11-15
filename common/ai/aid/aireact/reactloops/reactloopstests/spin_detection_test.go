package reactloopstests

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// TestIsInSameActionTypeSpin 测试 IsInSameActionTypeSpin 方法
// 构造一个自定义 action，然后反复调用，检查是否触发 SPIN 检测
func TestIsInSameActionTypeSpin(t *testing.T) {
	// 创建测试框架，设置较低的阈值以便快速触发
	framework := NewActionTestFrameworkEx(
		t,
		"spin-test",
		[]reactloops.ReActLoopOption{
			reactloops.WithSameActionTypeSpinThreshold(3), // 连续 3 次相同 Action 触发
			reactloops.WithEnableSelfReflection(true),
		},
		nil,
	)

	loop := framework.GetLoop()

	// 注册一个测试 action
	testActionName := "test_spin_action"
	framework.RegisterTestAction(
		testActionName,
		"Test action for spin detection",
		nil, // 无验证器
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			// 简单的 action handler，只是继续执行
			op.Continue()
		},
	)

	// 执行 4 次相同的 action（超过阈值 3）
	for i := 0; i < 4; i++ {
		err := framework.ExecuteAction(testActionName, map[string]interface{}{
			"iteration": i + 1,
		})
		if err != nil {
			t.Fatalf("ExecuteAction failed at iteration %d: %v", i+1, err)
		}
	}

	// 检查是否检测到 SPIN
	isSpinning := loop.IsInSameActionTypeSpin()
	if !isSpinning {
		t.Error("Expected IsInSameActionTypeSpin to return true after 4 consecutive same actions, but got false")
	}

	// 验证 action 历史记录
	allActions := loop.GetAllExistedActionRecord()
	if len(allActions) < 4 {
		t.Errorf("Expected at least 4 action records, got %d", len(allActions))
	}

	// 验证最后 3 个 action 都是相同类型
	last3Actions := loop.GetLastNAction(3)
	if len(last3Actions) != 3 {
		t.Errorf("Expected 3 last actions, got %d", len(last3Actions))
	}

	firstActionType := last3Actions[0].ActionType
	for i, action := range last3Actions {
		if action.ActionType != firstActionType {
			t.Errorf("Action at index %d has different type: expected %s, got %s", i, firstActionType, action.ActionType)
		}
	}
}

// TestIsInSameLogicSpinWithAI 测试 IsInSameLogicSpinWithAI 方法
// 需要检查 Timeline 中是否有 logic_spin_warning 条目
func TestIsInSameLogicSpinWithAI(t *testing.T) {
	var timelineEntries []string
	var spinWarningFound bool

	// 先定义 AI callback，现在 SPIN 检测整合到自我反思中
	aiCallback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		// 检查是否是自我反思的调用（包含 SPIN 检测数据）
		if strings.Contains(prompt, "SELF_REFLECTION_TASK") {
			rsp := i.NewAIResponse()
			// 检查是否包含 SPIN 检测数据
			if strings.Contains(prompt, "SPIN_DETECTION") {
				// 返回包含 SPIN 检测结果的自我反思结果
				reflectionResult := map[string]interface{}{
					"@action":     "self_reflection",
					"is_spinning": true,
					"spin_reason": "检测到连续执行相同的 Action，没有推进任务",
					"suggestions": []string{"尝试使用不同的 Action 类型", "检查任务目标是否明确", "尝试不同的策略", "重新评估任务目标"},
				}
				resultJSON, _ := json.Marshal(reflectionResult)
				rsp.EmitOutputStream(strings.NewReader(string(resultJSON)))
				rsp.Close()
				return rsp, nil
			}
			// 普通自我反思，不包含 SPIN 检测
			reflectionResult := map[string]interface{}{
				"@action": "self_reflection",
			}
			resultJSON, _ := json.Marshal(reflectionResult)
			rsp.EmitOutputStream(strings.NewReader(string(resultJSON)))
			rsp.Close()
			return rsp, nil
		}

		// 其他调用返回 finish action
		rsp := i.NewAIResponse()
		actionJSON := `{"@action": "finish", "answer": "Test completed"}`
		rsp.EmitOutputStream(strings.NewReader(actionJSON))
		rsp.Close()
		return rsp, nil
	}

	// 创建测试框架，设置较低的阈值
	framework := NewActionTestFrameworkEx(
		t,
		"spin-ai-test",
		[]reactloops.ReActLoopOption{
			reactloops.WithSameActionTypeSpinThreshold(2), // 连续 2 次触发简单检测
			reactloops.WithSameLogicSpinThreshold(2),      // 连续 2 次触发 AI 检测（现在整合到自我反思中）
			reactloops.WithEnableSelfReflection(true),
		},
		[]aicommon.ConfigOption{
			aicommon.WithAICallback(aiCallback),
			aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		},
	)

	loop := framework.GetLoop()

	// 注册一个测试 action
	testActionName := "test_ai_spin_action"
	framework.RegisterTestAction(
		testActionName,
		"Test action for AI spin detection",
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			op.Continue()
		},
	)

	// 捕获 Timeline 条目
	reactIns := framework.reactInstance
	if react, ok := reactIns.(*aireact.ReAct); ok {
		// 获取 Timeline 以便后续检查
		// 这里我们需要在执行后检查 Timeline
		_ = react
	}

	// 执行 3 次相同的 action（超过阈值 2）
	for i := 0; i < 3; i++ {
		err := framework.ExecuteAction(testActionName, map[string]interface{}{
			"iteration": i + 1,
		})
		if err != nil {
			t.Fatalf("ExecuteAction failed at iteration %d: %v", i+1, err)
		}
	}

	// 检查 Timeline 中是否有 logic_spin_warning
	// 由于 Timeline 是通过 invoker.AddToTimeline 添加的，我们需要检查
	// 这里我们通过检查 invoker 的 Timeline 来验证
	if react, ok := reactIns.(*aireact.ReAct); ok {
		config := react.GetConfig()
		if config != nil {
			if cfg, ok := config.(interface{ GetTimeline() *aicommon.Timeline }); ok {
				timeline := cfg.GetTimeline()
				if timeline != nil {
					outputs := timeline.ToTimelineItemOutputLastN(50)
					for _, output := range outputs {
						timelineEntries = append(timelineEntries, output.Content)
						if strings.Contains(output.Content, "logic_spin_warning") ||
							strings.Contains(output.Content, "SPIN DETECTED") ||
							strings.Contains(output.Content, "检测到 AI Agent 陷入循环") {
							spinWarningFound = true
						}
					}
				}
			}
		}
	}

	if !spinWarningFound {
		t.Logf("Timeline entries: %v", timelineEntries)
		t.Error("Expected to find logic_spin_warning in timeline, but not found")
	} else {
		t.Log("✅ Successfully found logic_spin_warning in timeline")
	}

	// 验证 IsInSpin 方法
	isSpinning, result := loop.IsInSpin()
	if !isSpinning {
		t.Error("Expected IsInSpin to return true, but got false")
	}
	if result == nil {
		t.Error("Expected IsInSpin to return a result, but got nil")
	} else {
		if !result.IsSpinning {
			t.Error("Expected result.IsSpinning to be true, but got false")
		}
		if result.Reason == "" {
			t.Error("Expected result.Reason to be non-empty, but got empty")
		}
		if len(result.Suggestions) == 0 {
			t.Error("Expected result.Suggestions to be non-empty, but got empty")
		}
	}
}

// TestSpinDetectionWithDifferentActions 测试不同 Action 不会触发 SPIN
func TestSpinDetectionWithDifferentActions(t *testing.T) {
	framework := NewActionTestFrameworkEx(
		t,
		"spin-different-test",
		[]reactloops.ReActLoopOption{
			reactloops.WithSameActionTypeSpinThreshold(3),
			reactloops.WithEnableSelfReflection(true),
		},
		nil,
	)

	loop := framework.GetLoop()

	// 注册两个不同的 action
	framework.RegisterTestAction(
		"action_a",
		"First test action",
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			op.Continue()
		},
	)

	framework.RegisterTestAction(
		"action_b",
		"Second test action",
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			op.Continue()
		},
	)

	// 交替执行不同的 action
	actions := []string{"action_a", "action_b", "action_a", "action_b"}
	for i, actionName := range actions {
		err := framework.ExecuteAction(actionName, map[string]interface{}{
			"iteration": i + 1,
		})
		if err != nil {
			t.Fatalf("ExecuteAction failed at iteration %d: %v", i+1, err)
		}
	}

	// 不应该检测到 SPIN（因为 Action 类型不同）
	isSpinning := loop.IsInSameActionTypeSpin()
	if isSpinning {
		t.Error("Expected IsInSameActionTypeSpin to return false for different actions, but got true")
	}

	isSpinningOverall, _ := loop.IsInSpin()
	if isSpinningOverall {
		t.Error("Expected IsInSpin to return false for different actions, but got true")
	}
}
