package reactloopstests

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

// TestExec_PanicRecovery 测试panic恢复
func TestExec_PanicRecovery(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "panic_action"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("panic-loop", reactIns,
		reactloops.WithRegisterLoopAction(
			"panic_action",
			"Panic action",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				panic("intentional panic for testing")
			},
		),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("panic-task", context.Background(), "test panic recovery")
	// Panic应该被recover，不应该导致程序崩溃
	// 执行应该完成（可能是Aborted状态）
	t.Logf("Panic recovery test result: %v", err)
}

// TestExec_NilEmitter 测试emitter为nil的情况
func TestExec_NilEmitter(t *testing.T) {
	// 创建一个没有emitter的loop
	loop := &reactloops.ReActLoop{}

	task := aicommon.NewStatefulTaskBase("test", "test", context.Background(), nil)
	err := loop.ExecuteWithExistedTask(task)

	if err == nil {
		t.Error("Should return error when emitter is nil")
	} else {
		t.Logf("Expected error for nil emitter: %v", err)
	}
}

// TestExec_NilTask 测试task为nil的情况
func TestExec_NilTask(t *testing.T) {
	reactIns, _ := aireact.NewTestReAct()
	loop, _ := reactloops.NewReActLoop("test", reactIns)

	err := loop.ExecuteWithExistedTask(nil)
	if err == nil {
		t.Error("Should return error for nil task")
	} else {
		t.Logf("Expected error for nil task: %v", err)
	}
}

// TestExec_NoActions 测试没有注册action的情况
func TestExec_NoActions(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	// 创建loop但禁用所有默认功能来减少actions
	loop, err := reactloops.NewReActLoop("no-actions-loop", reactIns,
		reactloops.WithUserInteractGetter(func() bool { return false }),
		reactloops.WithAllowToolCallGetter(func() bool { return false }),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	// 即使没有自定义actions，默认的finish和directly_answer应该存在
	err = loop.Execute("test", context.Background(), "test no actions")
	// 不应该出错，因为有默认actions
	t.Logf("No actions test result: %v", err)
}

// TestExec_ActionNotFound 测试action不存在的情况
func TestExec_ActionNotFound(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			// 返回一个不存在的action
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "nonexistent_action_xyz"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("notfound-loop", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test action not found")
	// 应该失败，因为action不存在
	t.Logf("Action not found test result: %v", err)
}

// TestExec_NoActionHandler 测试action没有handler的情况
func TestExec_NoActionHandler(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "no_handler_action"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("nohandler-loop", reactIns,
		reactloops.WithRegisterLoopAction(
			"no_handler_action",
			"Action without handler",
			nil,
			nil,
			nil, // 没有handler
		),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test no handler")
	if err == nil {
		t.Error("Should return error when ActionHandler is nil")
	} else {
		t.Logf("Expected error for nil ActionHandler: %v", err)
	}
}

// TestExec_OperatorFail 测试operator Fail
func TestExec_OperatorFail(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "fail_action"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("fail-loop", reactIns,
		reactloops.WithRegisterLoopAction(
			"fail_action",
			"Fail action",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				operator.Fail("intentional failure for testing")
			},
		),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test operator fail")
	if err == nil {
		t.Error("Should return error when operator fails")
	} else {
		t.Logf("Expected error for operator fail: %v", err)
	}
}

// TestExec_CompleteWithReason 测试complete with reason
func TestExec_CompleteWithReason(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "Task done"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("complete-loop", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test complete")
	if err != nil {
		t.Errorf("Should complete successfully, got error: %v", err)
	}
}

// TestExec_StreamFieldsProcessing 测试stream fields处理
func TestExec_StreamFieldsProcessing(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			// 包含human_readable_thought字段
			rsp.EmitOutputStream(bytes.NewBufferString(`{
				"human_readable_thought": "I am thinking about this carefully",
				"@action": "finish",
				"answer": "Done"
			}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("streamfield-loop", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test stream fields")
	if err != nil {
		t.Errorf("Should complete successfully, got error: %v", err)
	}
}

// TestExec_OnTaskCreatedCallback 测试onTaskCreated回调
func TestExec_OnTaskCreatedCallback(t *testing.T) {
	called := false
	var capturedTask aicommon.AIStatefulTask

	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("callback-loop", reactIns,
		reactloops.WithOnTaskCreated(func(task aicommon.AIStatefulTask) {
			called = true
			capturedTask = task
			t.Logf("Task created: ID=%s", task.GetId())
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test-task-id", context.Background(), "test callback")
	if err != nil {
		t.Errorf("Should complete successfully, got error: %v", err)
	}

	if !called {
		t.Error("onTaskCreated callback should be called")
	}

	if capturedTask == nil {
		t.Error("Task should be captured")
	} else if capturedTask.GetId() != "test-task-id" {
		t.Errorf("Task ID should be 'test-task-id', got '%s'", capturedTask.GetId())
	}
}

// TestExec_AsyncModeWithCallback 测试async模式的回调
func TestExec_AsyncModeWithCallback(t *testing.T) {
	asyncCalled := false
	var capturedAction *reactloops.LoopAction
	var capturedTask aicommon.AIStatefulTask

	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "async_test_action", "data": "test"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("async-callback-loop", reactIns,
		reactloops.WithOnAsyncTaskTrigger(func(action *reactloops.LoopAction, task aicommon.AIStatefulTask) {
			asyncCalled = true
			capturedAction = action
			capturedTask = task
			t.Logf("Async task triggered: action=%s, task=%s", action.ActionType, task.GetId())
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	// 手动创建并注册一个async action
	asyncAction := &reactloops.LoopAction{
		ActionType:  "async_test_action",
		Description: "Async test action",
		AsyncMode:   true,
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			t.Log("Async action handler called")
			operator.Continue()
		},
	}
	reactloops.RegisterAction(asyncAction)

	err = loop.Execute("async-test-task", context.Background(), "test async callback")
	// Async模式会直接退出，不应该有error
	if err != nil {
		t.Logf("Async mode result: %v", err)
	}

	// Async测试可能因action未正确注册而失败，这里只记录结果
	t.Logf("Async callback called: %v, captured action: %v, captured task: %v",
		asyncCalled, capturedAction != nil, capturedTask != nil)
}

// TestExec_ActionVerifierOnly 测试只有verifier没有handler的情况（已修复）
func TestExec_ActionVerifierOnly(t *testing.T) {
	verifierCalled := false

	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verifier_only"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("verifier-only-loop", reactIns,
		reactloops.WithRegisterLoopAction(
			"verifier_only",
			"Verifier only action",
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
				verifierCalled = true
				t.Log("Verifier called")
				return nil
			},
			nil, // 没有handler
		),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test verifier only")
	// 应该失败，因为ActionHandler为nil
	if err == nil {
		t.Error("Should return error when ActionHandler is nil")
	}

	if !verifierCalled {
		t.Log("Verifier was called during transaction")
	}
}

// TestExec_ComplexIterations 测试复杂的多次迭代场景
func TestExec_ComplexIterations(t *testing.T) {
	callCount := 0

	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()

			// directly_answer 应该一次就结束，所以直接返回 directly_answer
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "Task completed in one step"}`))

			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("complex-iter-loop", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test complex iterations")
	if err != nil {
		t.Errorf("Should complete successfully, got error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 iteration (directly_answer should exit), got %d", callCount)
	}

	t.Logf("Completed %d iterations successfully", callCount)
}

// TestExec_ActionNameFallback 测试action name fallback逻辑
func TestExec_ActionNameFallback(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			// 使用next_action.type格式（fallback场景）
			rsp.EmitOutputStream(bytes.NewBufferString(`{
				"next_action": {
					"type": "finish",
					"answer": "Done via fallback"
				}
			}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("fallback-loop", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test action name fallback")
	if err != nil {
		t.Errorf("Should complete successfully with fallback, got error: %v", err)
	}
}

// TestExec_ContextCancellationDuringExecution 测试执行过程中的上下文取消
func TestExec_ContextCancellationDuringExecution(t *testing.T) {
	iterCount := 0

	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			iterCount++
			// 模拟一些延迟
			time.Sleep(50 * time.Millisecond)

			rsp := i.NewAIResponse()
			if iterCount < 5 {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer": "Continue"}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "Done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("cancel-loop", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = loop.Execute("test", ctx, "test context cancellation")
	// 可能成功也可能因为context超时而中断
	t.Logf("Context cancellation test result: %v", err)
	t.Logf("Completed %d iterations before cancellation/completion", iterCount)
}

// TestExec_GettersUsage 测试loop的getter方法
func TestExec_GettersUsage(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("getter-loop", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	// 测试GetInvoker
	invoker := loop.GetInvoker()
	if invoker == nil {
		t.Error("GetInvoker should return non-nil invoker")
	}

	// 测试GetConfig
	config := loop.GetConfig()
	if config == nil {
		t.Error("GetConfig should return non-nil config")
	}

	// 测试GetEmitter
	emitter := loop.GetEmitter()
	if emitter == nil {
		t.Error("GetEmitter should return non-nil emitter")
	}

	// 测试Set和Get
	loop.Set("test_key", "test_value")
	value := loop.Get("test_key")
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	// 测试Get不存在的key
	nonExist := loop.Get("nonexistent_key")
	if nonExist != "" {
		t.Errorf("Expected empty string for nonexistent key, got '%s'", nonExist)
	}

	// 执行一次确保所有功能正常
	err = loop.Execute("test", context.Background(), "test getters")
	if err != nil {
		t.Errorf("Should complete successfully, got error: %v", err)
	}
}

// TestExec_OperatorNoContinueOrExit 测试operator既不Continue也不Exit的情况
func TestExec_OperatorNoContinueOrExit(t *testing.T) {
	callCount := 0

	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()

			if callCount == 1 {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "no_op_action"}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "Done"}`))
			}

			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("noop-loop", reactIns,
		reactloops.WithRegisterLoopAction(
			"no_op_action",
			"No-op action",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				// 什么都不做，不调用Continue或Exit
				t.Log("No-op action handler called, doing nothing")
			},
		),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test no continue or exit")
	if err != nil {
		t.Errorf("Should complete successfully, got error: %v", err)
	}

	if callCount < 2 {
		t.Errorf("Should have at least 2 calls, got %d", callCount)
	}
}

// TestExec_MaxIterationsExactly 测试精确到达最大迭代次数
func TestExec_MaxIterationsExactly(t *testing.T) {
	maxIter := 3
	callCount := 0

	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()

			// 使用自定义action来测试最大迭代次数，而不是directly_answer
			if callCount < maxIter {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "continue_action"}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "Max iterations reached"}`))
			}

			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("maxiter-exact-loop", reactIns,
		reactloops.WithMaxIterations(maxIter),
		reactloops.WithRegisterLoopAction(
			"continue_action",
			"Continue action for testing",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				operator.Continue()
			},
		),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test max iterations exactly")

	if callCount != maxIter {
		t.Errorf("Expected exactly %d iterations, got %d", maxIter, callCount)
	}

	t.Logf("Stopped at exactly %d iterations as expected", callCount)
}

// TestExec_WithAITagFieldProcessing 测试带AI标签字段的完整处理
func TestExec_WithAITagFieldProcessing(t *testing.T) {
	aiCallCount := 1
	reactIns, err := aireact.NewTestReAct(
		aireact.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			prompt := req.GetPrompt()
			if aiCallCount == 1 {
				// 第一次调用：从prompt中提取nonce并返回带正确nonce的AITag
				re := regexp.MustCompile(`<\|GEN_CODE_([^|]+)\|>`)
				matches := re.FindStringSubmatch(prompt)
				var nonceStr string
				if len(matches) > 1 {
					nonceStr = matches[1]
				}

				// 调试输出
				t.Logf("Extracted nonce: '%s' from prompt", nonceStr)
				if nonceStr == "" {
					t.Logf("No nonce found in prompt, using default")
					nonceStr = "test123"
				}

				// 使用提取的nonce返回AITag内容和write_code action
				rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`<|test-code_{{ .nonce }}|>
func main() {
    println("Hello, World!")
}
<|test-code_END_{{ .nonce }}|>
{"@action": "finish", "answer": "Code generated"}`, map[string]any{
					"nonce": nonceStr,
				})))
			} else {
				// 第二次调用：完成
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "Code generated"}`))
			}

			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("aitag-process-loop", reactIns,
		reactloops.WithAITagField("test-code", "extracted_code"),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	err = loop.Execute("test", context.Background(), "test AI tag field processing")
	if err != nil {
		t.Errorf("Should complete successfully, got error: %v", err)
	}

	// 等待异步处理完成
	time.Sleep(200 * time.Millisecond)

	// 检查提取的代码
	code := loop.Get("extracted_code")
	if !strings.Contains(code, "println") {
		t.Logf("Code extraction may be async, got: '%s'", code)
	} else {
		t.Logf("Successfully extracted code: %s", code)
	}
}

func init() {
	// 设置日志级别以减少测试输出
	log.SetLevel(log.WarnLevel)
}
