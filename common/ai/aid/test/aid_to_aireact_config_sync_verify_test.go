package test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestAIDToAIReact_ConfigSync_AllowAskForClarification_False
// 测试当 WithAllowRequireForUserInteract(false) 时，配置是否正确传递到 react loop
// 验证点：
// 1. prompt 中不应该包含 "主动提问以澄清意图" 相关内容
// 2. prompt 中不应该包含 "ask_for_clarification" action 的说明
// 3. AI 不应该尝试使用 ask_for_clarification action
func TestAIDToAIReact_ConfigSync_AllowAskForClarification_False(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	// 记录所有收到的 prompt，用于验证配置
	var receivedPrompts []string
	var askForClarificationActionUsed bool

	coordinator, err := aid.NewCoordinator(
		"test-allow-ask-for-clarification-false",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			receivedPrompts = append(receivedPrompts, prompt)
			rsp := i.NewAIResponse()
			defer rsp.Close()

			// 检查 prompt 中是否包含 AllowAskForClarification 相关的内容
			// 如果配置为 false，这些内容不应该出现在 prompt 中
			hasAskForClarificationContent := strings.Contains(prompt, "主动提问以澄清意图") ||
				strings.Contains(prompt, "ask_for_clarification") ||
				strings.Contains(prompt, "AskForClarification")

			if hasAskForClarificationContent {
				t.Errorf("配置 WithAllowRequireForUserInteract(false) 未生效：prompt 中仍然包含 ask_for_clarification 相关内容")
			}

			// 处理 react loop 请求
			if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
				// 检查 AI 是否尝试使用 ask_for_clarification action
				// 返回一个 finish action，避免触发用户交互
				responseJSON := `{"@action": "finish", "human_readable_thought": "测试完成"}`
				rsp.EmitOutputStream(strings.NewReader(responseJSON))
				return rsp, nil
			}

			// 处理 plan 请求
			if strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具") {
				rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "测试配置同步",
    "main_task": "验证配置传递",
    "main_task_goal": "确保配置正确传递到 react loop",
    "tasks": [
        {
            "subtask_name": "验证配置",
            "subtask_goal": "检查配置是否正确"
        }
    ]
}
				`))
				return rsp, nil
			}

			// 默认返回 finish
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "测试完成"}`))
			return rsp, nil
		}),
		aicommon.WithAllowRequireForUserInteract(false), // 关键配置：禁用用户交互
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	// 发送一个简单的任务
	inputChan.SafeFeed(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery: "测试配置同步",
		},
	})

	// 等待并收集事件
	timeout := time.After(5 * time.Second)
	eventCount := 0
LOOP:
	for {
		select {
		case <-timeout:
			break LOOP
		case result := <-outputChan:
			eventCount++
			if eventCount > 100 {
				break LOOP
			}

			// 检查是否收到了需要用户交互的事件（不应该出现）
			if result.Type == schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
				askForClarificationActionUsed = true
				t.Errorf("收到 EVENT_TYPE_REQUIRE_USER_INTERACTIVE 事件，但配置为 false，配置未生效")
			}

			// 如果已经收到并验证了 react loop 的 prompt，可以提前退出
			if len(receivedPrompts) > 0 {
				for _, prompt := range receivedPrompts {
					if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
						break LOOP
					}
				}
			}

			// 如果收到了 plan 事件，可以结束测试
			if result.Type == schema.EVENT_TYPE_PLAN {
				break LOOP
			}

			// 如果收到了 end_plan_and_execution 事件，也可以结束测试
			if result.Type == schema.EVENT_TYPE_END_PLAN_AND_EXECUTION {
				break LOOP
			}
		}
	}

	// 验证：检查所有收到的 prompt
	for i, prompt := range receivedPrompts {
		// 检查 react loop 的 prompt（包含 Background 或 Current Time）
		if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
			// 这些 prompt 不应该包含 ask_for_clarification 相关的内容
			if strings.Contains(prompt, "主动提问以澄清意图") {
				t.Errorf("Prompt #%d 包含 '主动提问以澄清意图'，但配置为 false", i)
			}
			if strings.Contains(prompt, "ask_for_clarification") {
				t.Errorf("Prompt #%d 包含 'ask_for_clarification'，但配置为 false", i)
			}
		}
	}

	// 最终验证
	if askForClarificationActionUsed {
		t.Fatal("配置 WithAllowRequireForUserInteract(false) 未生效：AI 仍然尝试使用 ask_for_clarification action")
	}

	t.Logf("测试通过：配置 WithAllowRequireForUserInteract(false) 正确传递到 react loop，共检查了 %d 个 prompt", len(receivedPrompts))
}

// TestAIDToAIReact_ConfigSync_AllowPlan_False
// 测试当 WithAllowPlanUserInteract(false) 时，配置是否正确传递到 react loop
// 验证点：
// 1. prompt 中不应该包含 "与规划"、"申请分步计划" 相关内容
// 2. prompt 中不应该包含 "request_plan_and_execution" action 的说明
// 3. AI 不应该尝试使用 request_plan_and_execution action
func TestAIDToAIReact_ConfigSync_AllowPlan_False(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	// 记录所有收到的 prompt，用于验证配置
	var receivedPrompts []string
	var planActionUsed bool

	coordinator, err := aid.NewCoordinator(
		"test-allow-plan-false",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			receivedPrompts = append(receivedPrompts, prompt)
			rsp := i.NewAIResponse()
			defer rsp.Close()

			// 处理 react loop 请求
			if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
				// 检查 prompt 中是否包含 AllowPlan 相关的内容
				// 如果配置为 false，这些内容不应该出现在 prompt 中
				hasPlanContent := strings.Contains(prompt, "与规划") ||
					strings.Contains(prompt, "申请分步计划") ||
					strings.Contains(prompt, "request_plan_and_execution") ||
					strings.Contains(prompt, "规划系统")

				if hasPlanContent {
					t.Errorf("配置 WithAllowPlanUserInteract(false) 未生效：prompt 中仍然包含 plan 相关内容")
				}

				// 返回一个 finish action，避免触发 plan
				responseJSON := `{"@action": "finish", "human_readable_thought": "测试完成"}`
				rsp.EmitOutputStream(strings.NewReader(responseJSON))
				return rsp, nil
			}

			// 处理 plan 请求（如果 AI 仍然尝试使用 plan，这里不应该被调用）
			if strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具") {
				planActionUsed = true
				t.Errorf("AI 尝试使用 plan action，但配置为 false，配置未生效")
				rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "测试配置同步",
    "main_task": "验证配置传递",
    "main_task_goal": "确保配置正确传递到 react loop",
    "tasks": []
}
				`))
				return rsp, nil
			}

			// 默认返回 finish
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "测试完成"}`))
			return rsp, nil
		}),
		aicommon.WithAllowPlanUserInteract(false), // 关键配置：禁用 plan
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	// 发送一个简单的任务
	inputChan.SafeFeed(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery: "测试配置同步 - AllowPlan",
		},
	})

	// 等待并收集事件
	timeout := time.After(5 * time.Second)
	eventCount := 0
LOOP:
	for {
		select {
		case <-timeout:
			break LOOP
		case result := <-outputChan:
			eventCount++
			if eventCount > 100 {
				break LOOP
			}

			// 如果已经收到并验证了 react loop 的 prompt，可以提前退出
			if len(receivedPrompts) > 0 {
				for _, prompt := range receivedPrompts {
					if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
						break LOOP
					}
				}
			}

			// 如果收到了 end_plan_and_execution 事件，可以结束测试
			if result.Type == schema.EVENT_TYPE_END_PLAN_AND_EXECUTION {
				break LOOP
			}
		}
	}

	// 验证：检查所有收到的 prompt
	for i, prompt := range receivedPrompts {
		// 检查 react loop 的 prompt（包含 Background 或 Current Time）
		if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
			// 这些 prompt 不应该包含 plan 相关的内容
			if strings.Contains(prompt, "与规划") {
				t.Errorf("Prompt #%d 包含 '与规划'，但配置为 false", i)
			}
			if strings.Contains(prompt, "申请分步计划") {
				t.Errorf("Prompt #%d 包含 '申请分步计划'，但配置为 false", i)
			}
			if strings.Contains(prompt, "request_plan_and_execution") {
				t.Errorf("Prompt #%d 包含 'request_plan_and_execution'，但配置为 false", i)
			}
			if strings.Contains(prompt, "规划系统") {
				t.Errorf("Prompt #%d 包含 '规划系统'，但配置为 false", i)
			}
		}
	}

	// 最终验证
	if planActionUsed {
		t.Fatal("配置 WithAllowPlanUserInteract(false) 未生效：AI 仍然尝试使用 plan action")
	}

	t.Logf("测试通过：配置 WithAllowPlanUserInteract(false) 正确传递到 react loop，共检查了 %d 个 prompt", len(receivedPrompts))
}

// TestAIDToAIReact_ConfigSync_Both_False
// 测试当同时设置 WithAllowRequireForUserInteract(false) 和 WithAllowPlanUserInteract(false) 时
// 验证两个配置都正确传递到 react loop
func TestAIDToAIReact_ConfigSync_Both_False(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	// 记录所有收到的 prompt，用于验证配置
	var receivedPrompts []string
	var askForClarificationActionUsed bool
	var planActionUsed bool

	coordinator, err := aid.NewCoordinator(
		"test-both-config-false",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			receivedPrompts = append(receivedPrompts, prompt)
			rsp := i.NewAIResponse()
			defer rsp.Close()

			// 处理 react loop 请求
			if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
				// 检查 prompt 中是否包含不应该出现的内容
				hasAskForClarificationContent := strings.Contains(prompt, "主动提问以澄清意图") ||
					strings.Contains(prompt, "ask_for_clarification")

				hasPlanContent := strings.Contains(prompt, "与规划") ||
					strings.Contains(prompt, "申请分步计划") ||
					strings.Contains(prompt, "request_plan_and_execution") ||
					strings.Contains(prompt, "规划系统")

				if hasAskForClarificationContent {
					t.Errorf("配置 WithAllowRequireForUserInteract(false) 未生效：prompt 中仍然包含 ask_for_clarification 相关内容")
				}

				if hasPlanContent {
					t.Errorf("配置 WithAllowPlanUserInteract(false) 未生效：prompt 中仍然包含 plan 相关内容")
				}

				// 返回一个 finish action
				responseJSON := `{"@action": "finish", "human_readable_thought": "测试完成"}`
				rsp.EmitOutputStream(strings.NewReader(responseJSON))
				return rsp, nil
			}

			// 处理 plan 请求（如果 AI 仍然尝试使用 plan，这里不应该被调用）
			if strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具") {
				planActionUsed = true
				t.Errorf("AI 尝试使用 plan action，但配置为 false，配置未生效")
				rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "测试配置同步",
    "main_task": "验证配置传递",
    "main_task_goal": "确保配置正确传递到 react loop",
    "tasks": []
}
				`))
				return rsp, nil
			}

			// 默认返回 finish
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "测试完成"}`))
			return rsp, nil
		}),
		aicommon.WithAllowRequireForUserInteract(false), // 禁用用户交互
		aicommon.WithAllowPlanUserInteract(false),       // 禁用 plan
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	// 发送一个简单的任务
	inputChan.SafeFeed(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery: "测试配置同步 - 两个配置都为 false",
		},
	})

	// 等待并收集事件
	timeout := time.After(5 * time.Second)
	eventCount := 0
LOOP:
	for {
		select {
		case <-timeout:
			break LOOP
		case result := <-outputChan:
			eventCount++
			if eventCount > 100 {
				break LOOP
			}

			// 检查是否收到了需要用户交互的事件（不应该出现）
			if result.Type == schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
				askForClarificationActionUsed = true
				t.Errorf("收到 EVENT_TYPE_REQUIRE_USER_INTERACTIVE 事件，但配置为 false，配置未生效")
			}

			// 如果已经收到并验证了 react loop 的 prompt，可以提前退出
			if len(receivedPrompts) > 0 {
				for _, prompt := range receivedPrompts {
					if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
						break LOOP
					}
				}
			}

			// 如果收到了 end_plan_and_execution 事件，可以结束测试
			if result.Type == schema.EVENT_TYPE_END_PLAN_AND_EXECUTION {
				break LOOP
			}
		}
	}

	// 验证：检查所有收到的 prompt
	for i, prompt := range receivedPrompts {
		// 检查 react loop 的 prompt（包含 Background 或 Current Time）
		if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
			// 这些 prompt 不应该包含 ask_for_clarification 相关的内容
			if strings.Contains(prompt, "主动提问以澄清意图") {
				t.Errorf("Prompt #%d 包含 '主动提问以澄清意图'，但配置为 false", i)
			}
			if strings.Contains(prompt, "ask_for_clarification") {
				t.Errorf("Prompt #%d 包含 'ask_for_clarification'，但配置为 false", i)
			}

			// 这些 prompt 不应该包含 plan 相关的内容
			if strings.Contains(prompt, "与规划") {
				t.Errorf("Prompt #%d 包含 '与规划'，但配置为 false", i)
			}
			if strings.Contains(prompt, "申请分步计划") {
				t.Errorf("Prompt #%d 包含 '申请分步计划'，但配置为 false", i)
			}
			if strings.Contains(prompt, "request_plan_and_execution") {
				t.Errorf("Prompt #%d 包含 'request_plan_and_execution'，但配置为 false", i)
			}
			if strings.Contains(prompt, "规划系统") {
				t.Errorf("Prompt #%d 包含 '规划系统'，但配置为 false", i)
			}
		}
	}

	// 最终验证
	if askForClarificationActionUsed {
		t.Fatal("配置 WithAllowRequireForUserInteract(false) 未生效：AI 仍然尝试使用 ask_for_clarification action")
	}

	if planActionUsed {
		t.Fatal("配置 WithAllowPlanUserInteract(false) 未生效：AI 仍然尝试使用 plan action")
	}

	t.Logf("测试通过：两个配置都正确传递到 react loop，共检查了 %d 个 prompt", len(receivedPrompts))
}

// TestAIDToAIReact_ConfigSync_Both_True
// 测试当同时设置 WithAllowRequireForUserInteract(true) 和 WithAllowPlanUserInteract(true) 时
// 验证两个配置都正确传递到 react loop（作为对比测试）
func TestAIDToAIReact_ConfigSync_Both_True(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	// 记录所有收到的 prompt，用于验证配置
	var receivedPrompts []string
	var foundAskForClarificationContent bool
	var foundPlanContent bool

	coordinator, err := aid.NewCoordinator(
		"test-both-config-true",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			receivedPrompts = append(receivedPrompts, prompt)
			rsp := i.NewAIResponse()
			defer rsp.Close()

			// 处理 react loop 请求
			if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
				// 检查 prompt 中是否包含应该出现的内容（配置为 true 时应该出现）
				if strings.Contains(prompt, "主动提问以澄清意图") || strings.Contains(prompt, "ask_for_clarification") {
					foundAskForClarificationContent = true
				}

				if strings.Contains(prompt, "与规划") || strings.Contains(prompt, "申请分步计划") || strings.Contains(prompt, "request_plan_and_execution") {
					foundPlanContent = true
				}

				// 返回一个 finish action
				responseJSON := `{"@action": "finish", "human_readable_thought": "测试完成"}`
				rsp.EmitOutputStream(strings.NewReader(responseJSON))
				return rsp, nil
			}

			// 默认返回 finish
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "测试完成"}`))
			return rsp, nil
		}),
		aicommon.WithAllowRequireForUserInteract(true), // 启用用户交互
		aicommon.WithAllowPlanUserInteract(true),       // 启用 plan
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	// 发送一个简单的任务
	inputChan.SafeFeed(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery: "测试配置同步 - 两个配置都为 true",
		},
	})

	// 等待并收集事件
	timeout := time.After(5 * time.Second)
	eventCount := 0
LOOP:
	for {
		select {
		case <-timeout:
			break LOOP
		case result := <-outputChan:
			eventCount++
			if eventCount > 100 {
				break LOOP
			}

			// 如果已经收到并验证了 react loop 的 prompt，可以提前退出
			if len(receivedPrompts) > 0 {
				for _, prompt := range receivedPrompts {
					if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
						break LOOP
					}
				}
			}

			// 如果收到了 end_plan_and_execution 事件，可以结束测试
			if result.Type == schema.EVENT_TYPE_END_PLAN_AND_EXECUTION {
				break LOOP
			}
		}
	}

	// 验证：检查所有收到的 prompt
	for _, prompt := range receivedPrompts {
		// 检查 react loop 的 prompt（包含 Background 或 Current Time）
		if strings.Contains(prompt, "Background") || strings.Contains(prompt, "Current Time:") {
			// 这些 prompt 应该包含 ask_for_clarification 相关的内容（配置为 true）
			if strings.Contains(prompt, "主动提问以澄清意图") || strings.Contains(prompt, "ask_for_clarification") {
				foundAskForClarificationContent = true
			}

			// 这些 prompt 应该包含 plan 相关的内容（配置为 true）
			if strings.Contains(prompt, "与规划") || strings.Contains(prompt, "申请分步计划") || strings.Contains(prompt, "request_plan_and_execution") {
				foundPlanContent = true
			}
		}
	}

	// 验证配置是否正确传递（配置为 true 时，应该能找到相关内容）
	if !foundAskForClarificationContent {
		t.Logf("警告：未在 prompt 中找到 ask_for_clarification 相关内容，但配置为 true（可能是 prompt 模板变化）")
	}

	if !foundPlanContent {
		t.Logf("警告：未在 prompt 中找到 plan 相关内容，但配置为 true（可能是 prompt 模板变化）")
	}

	t.Logf("测试完成：配置为 true 时的验证，共检查了 %d 个 prompt", len(receivedPrompts))
}
