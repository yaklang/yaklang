package aireact

import (
	"bytes"
	"fmt"
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
