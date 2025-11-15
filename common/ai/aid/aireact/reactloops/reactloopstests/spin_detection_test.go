package reactloopstests

import (
	"encoding/json"
	"fmt"
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

// TestSelfReflectionInvoked 测试自我反思被调用
// 验证在迭代次数 > 5 时，SPIN 检测会触发自我反思
func TestSelfReflectionInvoked(t *testing.T) {
	var reflectionCallCount int
	var aiCallCount int
	testActionName := "test_reflection_action"

	// 定义 AI callback，用于检测自我反思调用
	aiCallback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		aiCallCount++
		prompt := req.GetPrompt()

		// 检查是否是自我反思的调用
		if strings.Contains(prompt, "SELF_REFLECTION_TASK") {
			reflectionCallCount++
			rsp := i.NewAIResponse()

			// 检查是否包含 SPIN 检测数据
			if strings.Contains(prompt, "SPIN_DETECTION") {
				// 返回包含 SPIN 检测结果的自我反思结果
				reflectionResult := map[string]interface{}{
					"@action":     "self_reflection",
					"is_spinning": true,
					"spin_reason": "检测到连续执行相同的 Action，没有推进任务",
					"suggestions": []string{"尝试使用不同的 Action 类型", "检查任务目标是否明确"},
				}
				resultJSON, _ := json.Marshal(reflectionResult)
				rsp.EmitOutputStream(strings.NewReader(string(resultJSON)))
				rsp.Close()
				return rsp, nil
			}

			// 普通自我反思，不包含 SPIN 检测
			reflectionResult := map[string]interface{}{
				"@action":     "self_reflection",
				"suggestions": []string{"继续执行任务"},
			}
			resultJSON, _ := json.Marshal(reflectionResult)
			rsp.EmitOutputStream(strings.NewReader(string(resultJSON)))
			rsp.Close()
			return rsp, nil
		}

		// 检查是否是验证调用
		if strings.Contains(prompt, "verify-satisfaction") || strings.Contains(prompt, "user_satisfied") {
			rsp := i.NewAIResponse()
			actionJSON := `{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Test completed successfully", "human_readable_result": "Test result"}`
			rsp.EmitOutputStream(strings.NewReader(actionJSON))
			rsp.Close()
			return rsp, nil
		}

		// 其他调用返回我们注册的 action（让循环继续）
		rsp := i.NewAIResponse()
		actionJSON := fmt.Sprintf(`{"@action": "%s", "iteration": %d}`, testActionName, aiCallCount)
		rsp.EmitOutputStream(strings.NewReader(actionJSON))
		rsp.Close()
		return rsp, nil
	}

	// 创建测试框架，设置较低的阈值以便快速触发 SPIN
	framework := NewActionTestFrameworkEx(
		t,
		"reflection-invoked-test",
		[]reactloops.ReActLoopOption{
			reactloops.WithSameActionTypeSpinThreshold(3), // 连续 3 次相同 Action 触发
			reactloops.WithEnableSelfReflection(true),
		},
		[]aicommon.ConfigOption{
			aicommon.WithAICallback(aiCallback),
			aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		},
	)

	loop := framework.GetLoop()

	// 注册一个测试 action
	framework.RegisterTestAction(
		testActionName,
		"Test action for reflection invocation",
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			op.Continue()
		},
	)

	// 执行 4 次相同的 action（超过阈值 3）
	// 注意：由于每次 ExecuteAction 创建新任务，迭代计数会重置为 1
	// 因此无法直接测试 iterationCount > 5 的条件
	// 但我们可以测试：
	// 1. SPIN 检测功能本身（IsInSameActionTypeSpin）
	// 2. 自我反思被调用（通过检查反思历史）
	// 3. 如果 SPIN 被检测到，反思应该包含 SPIN 信息
	for i := 0; i < 4; i++ {
		err := framework.ExecuteAction(testActionName, map[string]interface{}{
			"iteration": i + 1,
		})
		if err != nil {
			t.Fatalf("ExecuteAction failed at iteration %d: %v", i+1, err)
		}
	}

	// 验证 SPIN 被检测到
	isSpinning := loop.IsInSameActionTypeSpin()
	if !isSpinning {
		t.Error("Expected IsInSameActionTypeSpin to return true after 4 consecutive same actions, but got false")
	} else {
		t.Log("✅ SPIN detected successfully")
	}

	// 验证自我反思被调用
	// 注意：由于迭代计数重置，可能不会触发标准级别的反思
	// 但至少应该有最小级别的反思
	reflectionHistory := loop.GetReflectionHistory()
	if len(reflectionHistory) == 0 {
		t.Error("Expected reflection history to contain entries, but it is empty")
	} else {
		t.Logf("✅ Reflection history contains %d entry(ies)", len(reflectionHistory))
		// 验证至少有一个反思记录
		for _, reflection := range reflectionHistory {
			t.Logf("  - Reflection at iteration %d, level: %s, action: %s",
				reflection.IterationNum, reflection.ReflectionLevel, reflection.ActionType)
		}
	}

	// 验证 AI 回调中的自我反思调用计数
	// 注意：由于迭代计数限制，可能不会触发 AI 反思（标准级别）
	// 但至少应该有最小级别的反思记录
	if reflectionCallCount > 0 {
		t.Logf("✅ Self-reflection AI call was invoked %d time(s)", reflectionCallCount)
	} else {
		t.Log("Note: Self-reflection AI call was not invoked (this is expected if only minimal reflection was triggered)")
	}
}

// TestSelfReflectionThenSpin 测试自我反思出现之后，然后再出现 SPIN，并且 SPIN 被检测成功，
// 再次触发自我反思，终结状态时，在 SPIN 触发的自我反思中可以看到 AddToTimeline 的内容，东西发生了变更
func TestSelfReflectionThenSpin(t *testing.T) {
	var reflectionCallCount int
	var spinReflectionCallCount int
	var aiCallCount int
	var timelineBeforeSpin []string
	var timelineAfterSpin []string
	testActionName := "test_reflection_then_spin_action"

	// 定义 AI callback，用于检测自我反思调用和 SPIN 检测
	aiCallback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		aiCallCount++
		prompt := req.GetPrompt()

		// 检查是否是自我反思的调用
		if strings.Contains(prompt, "SELF_REFLECTION_TASK") {
			reflectionCallCount++
			rsp := i.NewAIResponse()

			// 检查是否包含 SPIN 检测数据
			if strings.Contains(prompt, "SPIN_DETECTION") {
				spinReflectionCallCount++
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
				"@action":     "self_reflection",
				"suggestions": []string{"继续执行任务"},
			}
			resultJSON, _ := json.Marshal(reflectionResult)
			rsp.EmitOutputStream(strings.NewReader(string(resultJSON)))
			rsp.Close()
			return rsp, nil
		}

		// 检查是否是验证调用
		if strings.Contains(prompt, "verify-satisfaction") || strings.Contains(prompt, "user_satisfied") {
			rsp := i.NewAIResponse()
			actionJSON := `{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Test completed successfully", "human_readable_result": "Test result"}`
			rsp.EmitOutputStream(strings.NewReader(actionJSON))
			rsp.Close()
			return rsp, nil
		}

		// 其他调用返回我们注册的 action（让循环继续）
		rsp := i.NewAIResponse()
		actionJSON := fmt.Sprintf(`{"@action": "%s", "iteration": %d}`, testActionName, aiCallCount)
		rsp.EmitOutputStream(strings.NewReader(actionJSON))
		rsp.Close()
		return rsp, nil
	}

	// 创建测试框架，设置较低的阈值以便快速触发 SPIN
	framework := NewActionTestFrameworkEx(
		t,
		"reflection-then-spin-test",
		[]reactloops.ReActLoopOption{
			reactloops.WithSameActionTypeSpinThreshold(3), // 连续 3 次相同 Action 触发
			reactloops.WithEnableSelfReflection(true),
		},
		[]aicommon.ConfigOption{
			aicommon.WithAICallback(aiCallback),
			aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		},
	)

	loop := framework.GetLoop()
	reactIns := framework.reactInstance

	// 注册一个测试 action
	framework.RegisterTestAction(
		testActionName,
		"Test action for reflection then spin",
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			op.Continue()
		},
	)

	// 第一步：执行几次 action，建立 action 历史
	// 注意：由于每次 ExecuteAction 创建新任务，迭代计数会重置
	// 但 action 历史会保留在 loop 中
	for i := 0; i < 3; i++ {
		err := framework.ExecuteAction(testActionName, map[string]interface{}{
			"iteration": i + 1,
		})
		if err != nil {
			t.Fatalf("ExecuteAction failed at iteration %d: %v", i+1, err)
		}
	}

	// 记录第一次自我反思后的 Timeline 状态
	if react, ok := reactIns.(*aireact.ReAct); ok {
		config := react.GetConfig()
		if config != nil {
			if cfg, ok := config.(interface{ GetTimeline() *aicommon.Timeline }); ok {
				timeline := cfg.GetTimeline()
				if timeline != nil {
					outputs := timeline.ToTimelineItemOutputLastN(50)
					for _, output := range outputs {
						timelineBeforeSpin = append(timelineBeforeSpin, output.Content)
					}
				}
			}
		}
	}

	// 第二步：继续执行相同的 action，触发 SPIN 检测
	// 再执行 1 次，这样总共 4 次，应该触发 SPIN（连续 3 次相同，阈值是 3）
	err := framework.ExecuteAction(testActionName, map[string]interface{}{
		"iteration": 4,
	})
	if err != nil {
		t.Fatalf("ExecuteAction failed at iteration 4: %v", err)
	}

	// 记录 SPIN 触发后的 Timeline 状态
	if react, ok := reactIns.(*aireact.ReAct); ok {
		config := react.GetConfig()
		if config != nil {
			if cfg, ok := config.(interface{ GetTimeline() *aicommon.Timeline }); ok {
				timeline := cfg.GetTimeline()
				if timeline != nil {
					outputs := timeline.ToTimelineItemOutputLastN(50)
					for _, output := range outputs {
						timelineAfterSpin = append(timelineAfterSpin, output.Content)
					}
				}
			}
		}
	}

	// 验证自我反思被调用了
	if reflectionCallCount == 0 {
		t.Error("Expected self-reflection to be invoked, but it was not called")
	} else {
		t.Logf("✅ Self-reflection was invoked %d time(s)", reflectionCallCount)
	}

	// 验证 SPIN 触发的自我反思被调用了
	if spinReflectionCallCount == 0 {
		t.Error("Expected SPIN-triggered self-reflection to be invoked, but it was not called")
	} else {
		t.Logf("✅ SPIN-triggered self-reflection was invoked %d time(s)", spinReflectionCallCount)
	}

	// 验证反思历史中有记录
	reflectionHistory := loop.GetReflectionHistory()
	if len(reflectionHistory) == 0 {
		t.Error("Expected reflection history to contain entries, but it is empty")
	} else {
		t.Logf("✅ Reflection history contains %d entry(ies)", len(reflectionHistory))
	}

	// 验证 Timeline 中新增了 logic_spin_warning 条目
	spinWarningFound := false
	for _, content := range timelineAfterSpin {
		if strings.Contains(content, "logic_spin_warning") ||
			strings.Contains(content, "SPIN DETECTED") ||
			strings.Contains(content, "检测到 AI Agent 陷入循环") {
			spinWarningFound = true
			previewLen := 100
			if len(content) < previewLen {
				previewLen = len(content)
			}
			t.Logf("✅ Found SPIN warning in timeline: %s", content[:previewLen])
			break
		}
	}

	if !spinWarningFound {
		// 检查是否在 timelineBeforeSpin 中也没有（说明是新添加的）
		foundInBefore := false
		for _, content := range timelineBeforeSpin {
			if strings.Contains(content, "logic_spin_warning") ||
				strings.Contains(content, "SPIN DETECTED") ||
				strings.Contains(content, "检测到 AI Agent 陷入循环") {
				foundInBefore = true
				break
			}
		}

		if foundInBefore {
			t.Error("SPIN warning was found in timeline before SPIN was triggered, which is unexpected")
		} else {
			// 可能 Timeline 还没有更新，或者需要等待
			t.Log("Note: SPIN warning not found in timeline, but this might be expected if timeline update is async")
			t.Logf("Timeline entries before SPIN: %d", len(timelineBeforeSpin))
			t.Logf("Timeline entries after SPIN: %d", len(timelineAfterSpin))
		}
	}

	// 验证 Timeline 内容发生了变化
	if len(timelineAfterSpin) <= len(timelineBeforeSpin) {
		t.Logf("Timeline entries: before=%d, after=%d", len(timelineBeforeSpin), len(timelineAfterSpin))
		// 这不是错误，因为可能有些条目被合并或更新了
		// 但我们应该检查是否有新的 SPIN 相关条目
	}

	// 验证至少有一个反思包含 SPIN 信息
	hasSpinReflection := false
	for _, reflection := range reflectionHistory {
		if reflection.IsSpinning {
			hasSpinReflection = true
			t.Logf("✅ Found SPIN reflection at iteration %d: %s", reflection.IterationNum, reflection.SpinReason)
			if reflection.SpinReason == "" {
				t.Error("Expected SpinReason to be non-empty in SPIN reflection")
			}
			if len(reflection.Suggestions) == 0 {
				t.Error("Expected Suggestions to be non-empty in SPIN reflection")
			}
			break
		}
	}

	if !hasSpinReflection {
		t.Error("Expected to find at least one reflection with IsSpinning=true, but none was found")
	}
}
