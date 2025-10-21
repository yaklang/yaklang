package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		if toolName == "slow_task" {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 3.0 }}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.1 }}`))
		}
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
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
	
	// 创建慢任务工具
	slowTool, err := aitool.New(
		"slow_task",
		aitool.WithNumberParam("seconds"),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			slowToolCalled = true
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
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			// 根据用户输入决定使用哪个工具
			if utils.MatchAnyOfSubString(prompt, "run fast task") {
				return mockedToolCallingForJump(i, r, "fast_task")
			}
			// 默认使用 slow_task
			return mockedToolCallingForJump(i, r, "slow_task")
		}),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		WithTools(slowTool, fastTool),
		WithDebug(false),
		WithReviewPolicy(aicommon.AgreePolicyYOLO), // 跳过用户审核，直接执行工具
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
				jumpEventReceived = true
				jumpedTaskId := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.jumped_task_id"))
				fmt.Printf("Task jumped queue: %s\n", jumpedTaskId)
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

	fmt.Println("✅ Jump queue test passed successfully!")
}
