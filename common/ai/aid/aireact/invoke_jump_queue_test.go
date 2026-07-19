package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// mockedToolCallingForJump 模拟AI响应，用于插队测试
func mockedToolCallingForJump(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	// 用 isPrimaryDecisionPrompt 精确匹配主循环决策 prompt (而非宽泛子串匹配),
	// 避免 verification 之后的非主决策 prompt 误命中工具调用分支导致死循环.
	// verification 收缩为纯观测角色后, 工具调用过一次后的下一轮主循环决策会
	// 带着工具结果, 此时改返回 finish 收口 (模拟 "AI 判断任务完成后主动调
	// finish" 的新行为).
	//
	// 跨任务隔离: 插队场景下 slow_task 被取消后 fast_task 启动, fast_task 的
	// prompt 会带上 slow_task 的 timeline (含其 human_readable_thought /
	// ReAct iteration 标记). 所以用 prompt 内子串计数无法区分 "本任务已调过
	// 工具" 与 "上一任务残留 timeline". 这里改为外部传入的 *int32 toolCalled
	// 计数器 (由测试在 call-tool 时自增, 跨任务隔离), 用计数 >= 1 判定
	// "本任务至少调过一次工具". 默认传 nil 时回退到 prompt 子串检测 (单任务).
	return mockedToolCallingForJumpWithCounter(i, req, toolName, nil)
}

// mockedToolCallingForJumpWithCounter 是 mockedToolCallingForJump 的计数器版本.
// toolCalled 非 nil 时, 用其值检测 "本任务工具已调过"; 为 nil 时回退到 prompt
// 子串检测 (单任务场景).
func mockedToolCallingForJumpWithCounter(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string, toolCalled *int32) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if isPrimaryDecisionPrompt(prompt) {
		// verification 收缩为纯观测角色后, satisfied=true 不再自动退出. 工具调用
		// 过一轮后主动 finish 收口 (模拟 "AI 判断任务完成后主动调 finish" 的新行为).
		// 用外部传入的 toolCalled 计数器实现跨任务隔离 (插队场景 slow_task 残留
		// timeline 不会污染 fast_task 的判定).
		alreadyCalled := false
		if toolCalled != nil {
			alreadyCalled = atomic.LoadInt32(toolCalled) >= 1
		} else {
			alreadyCalled = strings.Count(prompt, "mocked thought for tool calling") >= 1
		}
		if alreadyCalled {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "mocked: task done after tool call"}`))
			rsp.Close()
			return rsp, nil
		}
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if isToolParamGenerationPrompt(prompt, toolName) {
		rsp := i.NewAIResponse()
		if toolName == "slow_task" {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 3.0 }}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.1 }}`))
		}
		rsp.Close()
		return rsp, nil
	}

	if isVerifySatisfactionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	// verification 收缩为纯观测角色后, satisfied=true 不再自动退出. 兜底返回
	// finish 收口 (模拟 "AI 判断任务完成后主动调 finish" 的新行为).
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "mocked post-iteration summary"}`))
	rsp.Close()
	return rsp, nil
}

// TestReAct_JumpQueue_StatusChanges 测试插队对任务状态的影响
func TestReAct_JumpQueue_StatusChanges(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	slowToolCalled := false
	fastToolCalled := false
	fastToolCompleted := false

	// verification 收缩为纯观测角色后, satisfied=true 不再自动退出. 跨任务隔离
	// 计数器: 用 atomic int32 跟踪每个任务调过工具的次数, 插队场景下 slow_task
	// 被取消后 fast_task 启动, 两者计数互不污染 (各自在工具 callback 里自增).
	var slowTaskCallCount int32
	var fastTaskCallCount int32

	// 创建慢任务工具
	slowTool, err := aitool.New(
		"slow_task",
		aitool.WithNumberParam("seconds"),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			slowToolCalled = true
			atomic.AddInt32(&slowTaskCallCount, 1)
			sleepDuration := params.GetFloat("seconds", 3.0)

			fmt.Printf("Slow task started, will run for %.1f seconds\n", sleepDuration)

			// 使用小的时间片来检测取消
			for i := 0; i < int(sleepDuration*10); i++ {
				select {
				case <-ctx.Done():
					fmt.Println("Slow task was cancelled")
					return nil, ctx.Err()
				case <-time.After(100 * time.Millisecond):
					// 继续执行
				}
			}

			fmt.Println("Slow task completed normally")
			return "slow task completed", nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create slow task tool: %v", err)
	}

	// 创建快任务工具
	fastTool, err := aitool.New(
		"fast_task",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			fastToolCalled = true
			atomic.AddInt32(&fastTaskCallCount, 1)
			sleepDuration := params.GetFloat("seconds", 0.1)
			time.Sleep(time.Duration(sleepDuration) * time.Second)
			fastToolCompleted = true
			fmt.Println("Fast task completed")
			return "fast task completed", nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create fast task tool: %v", err)
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			// 根据用户输入决定使用哪个工具
			if utils.MatchAnyOfSubString(prompt, "run fast task") {
				return mockedToolCallingForJumpWithCounter(i, r, "fast_task", &fastTaskCallCount)
			}
			// 默认使用 slow_task
			return mockedToolCallingForJumpWithCounter(i, r, "slow_task", &slowTaskCallCount)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(slowTool, fastTool),
		aicommon.WithDebug(false),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO), // 跳过用户审核，直接执行工具
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins

	go func() {
		// 发送第一个任务（慢任务）
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "run slow task",
		}

		// 稍微等待一下，然后发送第二个任务（快任务）
		time.Sleep(100 * time.Millisecond)
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "run fast task",
		}
	}()

	after := time.After(8 * time.Second)

	var task1Id, task2Id string
	task1Created := false
	task2Created := false
	task1Processing := false
	task1Cancelled := false
	task1Aborted := false
	task2Processing := false
	task2Completed := false
	jumpEventReceived := false
	slowToolStarted := false
	fastToolStarted := false

	syncId := ksuid.New().String()

LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())

			if e.NodeId == "react_task_created" {
				taskId := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_id"))
				if task1Id == "" {
					task1Id = taskId
					task1Created = true
					fmt.Printf("Task 1 created: %s\n", taskId)
				} else if task2Id == "" {
					task2Id = taskId
					task2Created = true
					fmt.Printf("Task 2 created: %s\n", taskId)
				}
			}

			if e.NodeId == "react_task_status_changed" {
				taskId := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_id"))
				status := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_now_status"))
				fmt.Printf("Task %s status changed to: %s\n", taskId, status)

				if taskId == task1Id {
					if status == "processing" {
						task1Processing = true
					} else if status == "aborted" {
						task1Aborted = true
					}
				} else if taskId == task2Id {
					if status == "processing" {
						task2Processing = true
					} else if status == "completed" {
						task2Completed = true
					}
				}
			}

			if e.Type == "tool_call_start" {
				toolName := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.tool.name"))
				fmt.Printf("Tool call started: %s\n", toolName)
				if toolName == "slow_task" {
					slowToolStarted = true
					fmt.Println("Slow tool started")

					// 等待一下确保工具开始执行，然后发送插队请求
					go func() {
						time.Sleep(200 * time.Millisecond)
						if task2Id != "" {
							fmt.Printf("Sending jump queue request for task: %s\n", task2Id)
							in <- &ypb.AIInputEvent{
								IsSyncMessage: true,
								SyncType:      SYNC_TYPE_REACT_JUMP_QUEUE,
								SyncJsonInput: fmt.Sprintf(`{"task_id": "%s"}`, task2Id),
								SyncID:        syncId,
							}
						}
					}()
				} else if toolName == "fast_task" {
					fastToolStarted = true
					fmt.Println("Fast tool started")
				}
			}

			if e.NodeId == "react_task_cancelled" {
				cancelledTaskId := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.task_id"))
				fmt.Printf("Task cancelled event received for task: %s\n", cancelledTaskId)
				if cancelledTaskId == task1Id {
					task1Cancelled = true
				}
			}

			if e.NodeId == "react_task_jumped_queue" {
				if e.SyncID == syncId {
					jumpEventReceived = true
					jumpedTaskId := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.jumped_task_id"))
					fmt.Printf("Task jumped queue: %s\n", jumpedTaskId)
				}
			}

			// 检查是否完成
			if task2Completed && jumpEventReceived && task1Cancelled {
				break LOOP
			}

		case <-after:
			break LOOP
		}
	}
	close(in)

	// 验证测试结果
	if !task1Created {
		t.Fatal("Expected task 1 to be created, but it wasn't")
	}
	if !task2Created {
		t.Fatal("Expected task 2 to be created, but it wasn't")
	}
	if !task1Processing {
		t.Fatal("Expected task 1 to enter processing state, but it didn't")
	}
	if !slowToolStarted {
		t.Fatal("Expected slow tool to start, but it didn't")
	}
	if !slowToolCalled {
		t.Fatal("Expected slow tool to be called, but it wasn't")
	}
	if !jumpEventReceived {
		t.Fatal("Expected jump queue event to be received, but it wasn't")
	}
	if !task1Cancelled {
		t.Fatal("Expected task 1 to be cancelled, but it wasn't")
	}
	if !task1Aborted {
		t.Fatal("Expected task 1 to be aborted, but it wasn't")
	}
	if !task2Processing {
		t.Fatal("Expected task 2 to enter processing state after jump, but it didn't")
	}
	if !fastToolStarted {
		t.Fatal("Expected fast tool to start after jump, but it didn't")
	}
	if !fastToolCalled {
		t.Fatal("Expected fast tool to be called after jump, but it wasn't")
	}
	if !fastToolCompleted {
		t.Fatal("Expected fast tool to complete, but it didn't")
	}
	if !task2Completed {
		t.Fatal("Expected task 2 to be completed, but it wasn't")
	}

	fmt.Println("Jump queue test passed successfully!")
}
