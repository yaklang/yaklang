package aireact

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"io"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// mockedToolCallingForRemove 模拟AI响应，用于移除测试
func mockedToolCallingForRemove(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "directly_answer", "answer_payload": "Task completed"}
`))
		rsp.Close()
		return rsp, nil
	}
	return nil, fmt.Errorf("unsupported prompt: %s", prompt)
}

func TestReAct_RemoveTask_StatusChanges(t *testing.T) {
	// 测试状态跟踪变量
	var (
		task1Id         string
		task2Id         string
		task3Id         string
		task1Removed    bool
		queueInfoSent   bool
		dequeueReceived bool
		reActFinished   bool
	)
	syncID := uuid.NewString()

	// 使用 CondBarrier 精确控制时序
	cb := utils.NewCondBarrier()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 100)

	ins, err := NewTestReAct(
		aicommon.WithAICallback(mockedToolCallingForRemove),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out <- e:
			case <-time.After(1 * time.Second):
				// 防止阻塞
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins

	timeout := time.After(10 * time.Second)

	// 发送三个任务
	go func() {
		// 发送第一个任务
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "task 1",
		}

		// 发送第二个任务
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "task 2",
		}

		// 发送第三个任务
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "task 3",
		}
	}()

	// 等待所有3个任务都进入 queueing 状态后再发送移除请求
	go func() {
		// 等待3个任务都创建并进入队列
		if err := cb.Wait("task1_queueing", "task2_queueing", "task3_queueing"); err != nil {
			t.Errorf("Failed to wait for tasks to queue: %v", err)
			return
		}

		// 确保在 task2 被 dequeue 之前发送移除请求
		// 短暂延迟确保状态稳定，但要在第一个任务开始处理之前
		time.Sleep(50 * time.Millisecond)

		if task2Id != "" {
			fmt.Printf("Sending remove request for task: %s\n", task2Id)
			in <- &ypb.AIInputEvent{
				IsSyncMessage: true,
				SyncType:      SYNC_TYPE_REACT_REMOVE_TASK,
				SyncJsonInput: fmt.Sprintf(`{"task_id": "%s"}`, task2Id),
				SyncID:        syncID,
			}
		}
	}()

eventLoop:
	for {
		select {
		case <-timeout:
			t.Fatal("Test timed out")
		case e := <-out:
			if e.Type == "structured" {
				switch e.NodeId {
				case "react_task_created":
					// 记录任务ID
					if content := string(e.Content); content != "" {
						if taskId := utils.InterfaceToString(jsonpath.FindFirst(content, "$.react_task_id")); taskId != "" {
							if task1Id == "" {
								task1Id = taskId
								fmt.Printf("Task 1 created: %s\n", taskId)
							} else if task2Id == "" {
								task2Id = taskId
								fmt.Printf("Task 2 created: %s\n", taskId)
							} else if task3Id == "" {
								task3Id = taskId
								fmt.Printf("Task 3 created: %s\n", taskId)
							}
						}
					}

				case "react_task_status_changed":
					// 检查任务状态变化
					if content := string(e.Content); content != "" {
						taskId := utils.InterfaceToString(jsonpath.FindFirst(content, "$.react_task_id"))
						status := utils.InterfaceToString(jsonpath.FindFirst(content, "$.react_task_now_status"))

						if status == "queueing" {
							// 标记任务已进入队列
							if taskId == task1Id {
								b := cb.CreateBarrier("task1_queueing")
								b.Done()
								fmt.Printf("Task 1 entered queueing state\n")
							} else if taskId == task2Id {
								b := cb.CreateBarrier("task2_queueing")
								b.Done()
								fmt.Printf("Task 2 entered queueing state\n")
							} else if taskId == task3Id {
								b := cb.CreateBarrier("task3_queueing")
								b.Done()
								fmt.Printf("Task 3 entered queueing state\n")
							}
						}

						if status != "" && taskId != "" {
							fmt.Printf("Task %s status changed to: %s\n", taskId, status)

							// 当第一个任务完成时，结束测试
							if taskId == task1Id && status == "completed" && task1Removed && queueInfoSent && dequeueReceived {
								reActFinished = true
								fmt.Println("ReAct processing finished")
								close(in)
								break eventLoop
							}
						}
					}

				case "react_task_dequeue":
					// 检查是否是移除事件
					if content := string(e.Content); content != "" {
						if reason := utils.InterfaceToString(jsonpath.FindFirst(content, "$.reason")); reason == "manual_remove" {
							if taskId := utils.InterfaceToString(jsonpath.FindFirst(content, "$.react_task_id")); taskId == task2Id {
								dequeueReceived = true
								task1Removed = true
								fmt.Printf("Task 2 dequeue event received: %s\n", taskId)
							}
						}
					}

				case "queue_info":
					// 检查队列信息更新
					if task1Removed && !queueInfoSent {
						if syncID != e.SyncID {
							t.Fatalf("Expected syncID %s but got %s in queue_info event", syncID, e.SyncID)
						}

						queueInfoSent = true
						fmt.Println("Queue info updated after task removal")

						// 验证队列内容
						if content := string(e.Content); content != "" {
							if queueData := jsonpath.FindFirst(content, "$.queue"); queueData != nil {
								fmt.Printf("Queue after removal: %v\n", queueData)
							}
						}
					}

				}
			}
		}
	}

	// 验证测试结果
	if !task1Removed {
		t.Fatal("Expected task to be removed, but it wasn't")
	}

	if !dequeueReceived {
		t.Fatal("Expected to receive dequeue event, but didn't")
	}

	if !queueInfoSent {
		t.Fatal("Expected to receive queue info update, but didn't")
	}

	if !reActFinished {
		t.Fatal("Expected ReAct to finish processing, but it didn't")
	}

	fmt.Println("[SUCCESS] Remove task test passed successfully!")
}

// TestReAct_Clear_StatusChanges 测试清理队列对任务的
func TestReAct_Clear_StatusChanges(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	// 创建慢任务工具
	slowTool, err := aitool.New(
		"slow_task",
		aitool.WithNumberParam("seconds"),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
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
			sleepDuration := params.GetFloat("seconds", 0.1)
			time.Sleep(time.Duration(sleepDuration) * time.Second)
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
				return mockedToolCallingForJump(i, r, "fast_task")
			}
			// 默认使用 slow_task
			return mockedToolCallingForJump(i, r, "slow_task")
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

	cb := utils.NewCondBarrierContext(context.Background())

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

		time.Sleep(100 * time.Millisecond)
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "run fast task",
		}
	}()

	after := time.After(8 * time.Second)

	var task1Id, task2Id, task3Id string
	clearEventReceived := false
	slowToolStarted := false
	queueInfoOk := false

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
					cb.CreateBarrier("task1").Done()
				} else if task2Id == "" {
					task2Id = taskId
					cb.CreateBarrier("task2").Done()
				} else if task3Id == "" {
					task3Id = taskId
					cb.CreateBarrier("task3").Done()
				}
			}

			if e.NodeId == "react_task_status_changed" {
				taskId := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_id"))
				status := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_now_status"))
				fmt.Printf("Task %s status changed to: %s\n", taskId, status)
			}

			if e.Type == "tool_call_start" {
				toolName := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.tool.name"))
				fmt.Printf("Tool call started: %s\n", toolName)
				if toolName == "slow_task" {
					slowToolStarted = true
					fmt.Println("Slow tool started")

					// 等待一下确保工具开始执行，然后发送插队请求
					go func() {
						err := cb.Wait("task1", "task2", "task3")
						if err != nil {
							return
						}
						time.Sleep(200 * time.Millisecond)
						if task2Id != "" {
							fmt.Printf("Sending jump queue request for task: %s\n", task2Id)
							in <- &ypb.AIInputEvent{
								IsSyncMessage: true,
								SyncType:      SYNC_TYPE_REACT_CLEAR_TASK,
								SyncID:        syncId,
							}
						}
					}()
				}
			}

			if e.NodeId == REACT_TASK_clear {
				if e.SyncID == syncId {
					clearEventReceived = true
				}
				in <- &ypb.AIInputEvent{
					IsSyncMessage: true,
					SyncType:      SYNC_TYPE_QUEUE_INFO,
				}
			}

			if clearEventReceived && e.NodeId == "queue_info" {
				if content := string(e.Content); content != "" {
					queueInfoOk = true
					if queueLen := jsonpath.FindFirst(content, "$.total_tasks"); queueLen != nil {
						require.Zero(t, queueLen)
					}
				}
			}

			if clearEventReceived && slowToolStarted && queueInfoOk {
				break LOOP
			}

		case <-after:
			break LOOP
		}
	}
	close(in)

	if !slowToolStarted {
		t.Fatal("Expected slow tool to start, but it didn't")
	}
	if !clearEventReceived {
		t.Fatal("Expected clear queue event to be received, but it wasn't")
	}
	if !queueInfoOk {
		t.Fatal("Expected queue info to be received, but it wasn't")
	}

	fmt.Println("✅ clear queue test passed successfully!")
}
