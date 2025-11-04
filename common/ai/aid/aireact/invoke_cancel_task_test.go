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

// mockedToolCallingForCancel 模拟AI响应，用于取消测试
func mockedToolCallingForCancel(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
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
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 2.0 }}`))
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

// TestReAct_CancelCurrentTask_StatusChanges 测试取消当前任务对状态的影响
func TestReAct_CancelCurrentTask_StatusChanges(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	toolCalled := false
	toolCompleted := false

	// 创建一个长时间运行的工具
	longTool, err := aitool.New(
		"long_task",
		aitool.WithNumberParam("seconds"),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			sleepDuration := params.GetFloat("seconds", 2.0)
			if sleepDuration <= 0 || sleepDuration > 2.0 {
				sleepDuration = 2.0
			}

			fmt.Printf("Long task started, will run for %.1f seconds\n", sleepDuration)

			// 使用小的时间片来检测取消
			for i := 0; i < int(sleepDuration*20); i++ {
				select {
				case <-ctx.Done():
					fmt.Println("Long task was cancelled")
					return nil, ctx.Err()
				case <-time.After(50 * time.Millisecond):
					// 继续执行
				}
			}

			toolCompleted = true
			fmt.Println("Long task completed normally")
			return "task completed", nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create long task tool: %v", err)
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingForCancel(i, r, "long_task")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(longTool),
		aicommon.WithDebug(false),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO), // 跳过用户审核，直接执行工具
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "run long task",
		}
	}()

	after := time.After(3 * time.Second)

	var taskId string
	taskCreated := false
	taskProcessing := false
	taskCancelled := false
	taskAborted := false
	toolStarted := false
	toolWatcherEmitted := false
	cancelEventReceived := false

LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())

			if e.NodeId == "react_task_created" {
				taskCreated = true
				taskId = utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_id"))
				fmt.Printf("Task created: %s\n", taskId)
			}

			if e.NodeId == "react_task_status_changed" {
				status := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_now_status"))
				fmt.Printf("Task status changed to: %s\n", status)

				if status == "processing" {
					taskProcessing = true
				} else if status == "aborted" {
					taskAborted = true
				}
			}

			if e.Type == "tool_call_start" {
				toolStarted = true
				fmt.Println("Tool call started")
			}

			// 等待 tool_call_watcher 事件，这表明工具即将真正执行
			if e.Type == "tool_call_watcher" {
				toolWatcherEmitted = true
				fmt.Println("Tool call watcher emitted, sending cancel request")
				// 立即发送取消请求
				go func() {
					in <- &ypb.AIInputEvent{
						IsSyncMessage: true,
						SyncType:      SYNC_TYPE_REACT_CANCEL_CURRENT_TASK,
					}
				}()
			}

			if e.NodeId == "react_task_cancelled" {
				cancelEventReceived = true
				cancelledTaskId := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.task_id"))
				fmt.Printf("Task cancelled event received for task: %s\n", cancelledTaskId)
				if cancelledTaskId == taskId {
					taskCancelled = true
				}
			}

			// 检查任务是否完成（被中止）
			if taskAborted && cancelEventReceived {
				break LOOP
			}

		case <-after:
			break LOOP
		}
	}
	close(in)

	// 验证测试结果
	if !taskCreated {
		t.Fatal("Expected task to be created, but it wasn't")
	}
	if !taskProcessing {
		t.Fatal("Expected task to enter processing state, but it didn't")
	}
	if !toolStarted {
		t.Fatal("Expected tool to start, but it didn't")
	}
	if !toolWatcherEmitted {
		t.Fatal("Expected tool watcher to be emitted, but it wasn't")
	}
	// toolCalled 可能为 false，因为取消可能在工具真正开始执行之前就发生了
	// 这是正常的，取消的目的就是尽快停止执行
	if toolCompleted {
		t.Fatal("Expected tool to be cancelled before completion, but it completed")
	}
	if !taskCancelled {
		t.Fatal("Expected task to be cancelled, but it wasn't")
	}
	if !cancelEventReceived {
		t.Fatal("Expected cancel event to be received, but it wasn't")
	}
	if !taskAborted {
		t.Fatal("Expected task to be aborted, but it wasn't")
	}

	fmt.Printf("✅ Cancel current task test passed successfully! (toolCalled: %v)\n", toolCalled)
}
