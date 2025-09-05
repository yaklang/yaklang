package aid

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func testRecoveryToolUseReview(t *testing.T, uid string) {
	fmt.Println("------------------------------------------------------------")
	fmt.Println("------------------------------------------------------------")
	fmt.Println("------------------------------------------------------------")
	fmt.Println("------------------------------------------------------------")
	fmt.Println("------------------------------------------------------------")
	inputChan := make(chan *InputEvent, 100)            // 增加缓冲区大小
	outputChan := make(chan *schema.AiOutputEvent, 100) // 增加缓冲区大小
	coordinator, err := NewFastRecoverCoordinator(
		uid,
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			select {
			case outputChan <- event:
			case <-time.After(1 * time.Second):
				// 防止阻塞，但记录警告
				fmt.Printf("Warning: output channel full, dropping event: %s\n", event.String())
			}
		}),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	var mu sync.Mutex
	useToolReview := false
	useToolReviewPass := false
	count := 0
	maxRetries := 3
	retryCount := 0

	// 增加重试机制
	for retryCount < maxRetries {
		timeout := time.After(10 * time.Second) // 增加超时时间
		eventCount := 0
		maxEvents := 200 // 增加最大事件数

	LOOP:
		for {
			select {
			case <-timeout:
				fmt.Printf("Timeout reached on retry %d\n", retryCount+1)
				break LOOP
			case result := <-outputChan:
				mu.Lock()
				count++
				eventCount++
				mu.Unlock()

				if eventCount > maxEvents {
					fmt.Printf("Max events reached on retry %d\n", retryCount+1)
					break LOOP
				}

				fmt.Println("result:" + result.String())
				if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
					select {
					case inputChan <- &InputEvent{
						Id: result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"suggestion": "continue",
						},
					}:
					case <-time.After(1 * time.Second):
						fmt.Println("Warning: input channel full, dropping plan review response")
					}
					continue
				}

				if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
					var a = make(aitool.InvokeParams)
					json.Unmarshal(result.Content, &a)
					if a.GetObject("params").GetString("path") == "/abc-target" &&
						a.GetString("tool") == "ls" && a.GetString("tool_description") != "" {
						mu.Lock()
						useToolReview = true
						mu.Unlock()

						select {
						case inputChan <- &InputEvent{
							Id: result.GetInteractiveId(),
							Params: aitool.InvokeParams{
								"suggestion": "continue",
							},
						}:
						case <-time.After(1 * time.Second):
							fmt.Println("Warning: input channel full, dropping tool review response")
						}
						continue
					}
				}

				mu.Lock()
				if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "ls") {
					useToolReviewPass = true
					mu.Unlock()
					break LOOP
				}
				mu.Unlock()

				fmt.Println("review task result:" + result.String())
			}
		}

		// 检查是否成功
		mu.Lock()
		success := useToolReview && useToolReviewPass
		mu.Unlock()

		if success {
			break
		}

		retryCount++
		if retryCount < maxRetries {
			fmt.Printf("Test failed on attempt %d, retrying...\n", retryCount)
			// 重置状态
			mu.Lock()
			useToolReview = false
			useToolReviewPass = false
			count = 0
			mu.Unlock()
			time.Sleep(2 * time.Second) // 等待一段时间再重试
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if !useToolReview {
		t.Fatal("tool review fail")
	}

	if !useToolReviewPass {
		t.Fatal("tool review not finished")
	}
}

func TestCoordinator_Recovery_ToolUseReview(t *testing.T) {
	inputChan := make(chan *InputEvent, 100)            // 增加缓冲区大小
	outputChan := make(chan *schema.AiOutputEvent, 100) // 增加缓冲区大小
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			select {
			case outputChan <- event:
			case <-time.After(1 * time.Second):
				// 防止阻塞，但记录警告
				fmt.Printf("Warning: output channel full, dropping event: %s\n", event.String())
			}
		}),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: ls`, `"call-tool"`, "const") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "ls", "params": {"path": "/abc-target"}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "ls"}`))
				return rsp, nil
			}
			// 处理决策阶段 - 检查更多的决策阶段特征
			if utils.MatchAllOfSubString(request.GetPrompt(), `review当前任务的执行情况`, `决策`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `刚使用了一个工具来帮助你完成任务`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `continue-current-task`, `proceed-next-task`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `task-failed`, `task-skipped`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `"enum": ["continue-current-task"`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `工具的结果如下，产生结果时间为`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "proceed-next-task"}`))
				return rsp, nil
			}

			// 如果没有匹配到特定的模式，但包含决策相关的关键字，返回默认决策
			if utils.MatchAnyOfSubString(request.GetPrompt(), `continue-current-task`, `proceed-next-task`, `task-failed`, `task-skipped`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "proceed-next-task"}`))
				return rsp, nil
			}

			fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())
			rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
    "main_task": "在给定路径下寻找体积最大的文件",
    "main_task_goal": "识别 /Users/v1ll4n/Projects/yaklang 目录中占用存储空间最多的文件，并展示其完整路径与大小信息",
    "tasks": [
        {
            "subtask_name": "扫描目录结构",
            "subtask_goal": "递归遍历 /Users/v1ll4n/Projects/yaklang 目录下所有文件，记录每个文件的位置和占用空间"
        },
        {
            "subtask_name": "计算文件大小",
            "subtask_goal": "遍历所有文件，计算每个文件的大小"
        }
    ]
}
			`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	var mu sync.Mutex
	useToolReview := false
	useToolReviewPass := false
	count := 0
	maxRetries := 3
	retryCount := 0

	// 增加重试机制
	for retryCount < maxRetries {
		timeout := time.After(15 * time.Second) // 增加超时时间
		eventCount := 0
		maxEvents := 200 // 增加最大事件数

	LOOP:
		for {
			select {
			case <-timeout:
				fmt.Printf("Timeout reached on retry %d\n", retryCount+1)
				break LOOP
			case result := <-outputChan:
				mu.Lock()
				count++
				eventCount++
				mu.Unlock()

				if eventCount > maxEvents {
					fmt.Printf("Max events reached on retry %d\n", retryCount+1)
					break LOOP
				}

				fmt.Println("result:" + result.String())
				if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
					select {
					case inputChan <- &InputEvent{
						Id: result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"suggestion": "continue",
						},
					}:
					case <-time.After(1 * time.Second):
						fmt.Println("Warning: input channel full, dropping plan review response")
					}
					continue
				}

				if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
					var a = make(aitool.InvokeParams)
					json.Unmarshal(result.Content, &a)
					if a.GetObject("params").GetString("path") == "/abc-target" &&
						a.GetString("tool") == "ls" && a.GetString("tool_description") != "" {
						mu.Lock()
						useToolReview = true
						mu.Unlock()

						select {
						case inputChan <- &InputEvent{
							Id: result.GetInteractiveId(),
							Params: aitool.InvokeParams{
								"suggestion": "continue",
							},
						}:
						case <-time.After(1 * time.Second):
							fmt.Println("Warning: input channel full, dropping tool review response")
						}
						continue
					}
				}

				mu.Lock()
				if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "ls") {
					useToolReviewPass = true
					mu.Unlock()
					break LOOP
				}
				mu.Unlock()

				fmt.Println("review task result:" + result.String())
			}
		}

		// 检查是否成功
		mu.Lock()
		success := useToolReview && useToolReviewPass
		mu.Unlock()

		if success {
			break
		}

		retryCount++
		if retryCount < maxRetries {
			fmt.Printf("Test failed on attempt %d, retrying...\n", retryCount)
			// 重置状态
			mu.Lock()
			useToolReview = false
			useToolReviewPass = false
			count = 0
			mu.Unlock()
			time.Sleep(2 * time.Second) // 等待一段时间再重试
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if !useToolReview {
		t.Fatal("tool review fail")
	}

	if !useToolReviewPass {
		t.Fatal("tool review not finished")
	}
	testRecoveryToolUseReview(t, coordinator.config.id)
}
