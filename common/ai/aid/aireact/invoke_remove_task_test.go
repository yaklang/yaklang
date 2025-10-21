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
{"@action": "object", "next_action": "directly_answer", "directly_answer": "Task completed"}
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

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 100)

	ins, err := NewTestReAct(
		WithAICallback(mockedToolCallingForRemove),
		WithReviewPolicy(aicommon.AgreePolicyYOLO),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
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

	timeout := time.After(5 * time.Second)

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

								// 等待一下让任务入队，然后移除第二个任务
								time.Sleep(200 * time.Millisecond)

								// 发送移除请求，移除 task2
								fmt.Printf("Sending remove request for task: %s\n", task2Id)
								in <- &ypb.AIInputEvent{
									IsSyncMessage: true,
									SyncType:      SYNC_TYPE_REACT_REMOVE_TASK,
									SyncJsonInput: fmt.Sprintf(`{"task_id": "%s"}`, task2Id),
								}
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

				case "react_task_status_changed":
					// 检查任务状态变化
					if content := string(e.Content); content != "" {
						if status := utils.InterfaceToString(jsonpath.FindFirst(content, "$.react_task_now_status")); status != "" {
							if taskId := utils.InterfaceToString(jsonpath.FindFirst(content, "$.react_task_id")); taskId != "" {
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

	fmt.Println("✅ Remove task test passed successfully!")
}
