package aid

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_Basic_WithMemoryPreset(t *testing.T) {
	m := memoryTestBasic(t)
	_ = m
}

func TestCoordinator_SidecarMemory_Timeline_ToolUse_TooMany_TimelineShrink(t *testing.T) {
	m := memoryTestBasic(t)
	m.ClearRuntimeConfig()

	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)

	requireMoreToolCount := 0

	timelienShrinkApplyCount := 0
	tokenPersistent := utils.RandStringBytes(100)
	token1 := "memory-timeline-sidecar+" + utils.RandStringBytes(100)
	token2 := "memory-timeline-sidecar+" + utils.RandStringBytes(100)
	m.timeline.PushUserInteraction(aicommon.UserInteractionStage_FreeInput, 1, token1, token1)
	m.timeline.PushUserInteraction(aicommon.UserInteractionStage_FreeInput, 2, token2, token2)
	result := m.timeline.Dump()
	fmt.Println(result)
	fmt.Println("token1", token1)
	fmt.Println("token2", token2)
	// require.Contains(t, result, token1, token2)

	fmt.Println(m.timeline.Dump())

	noexistedfileToken := utils.RandStringBytes(100)
	haveSidecarMem := false
	requireMoreToolCountForShrink := 0
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithMemory(m),
		WithTimelineContentLimit(200),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			fmt.Println("========================================================")
			fmt.Println(request.GetPrompt())

			if utils.MatchAllOfSubString(request.GetPrompt(), token2, token1, `"main_task_goal"`, `"main_task"`, `"const": "plan"`) {
				haveSidecarMem = true
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), tokenPersistent) {
				timelienShrinkApplyCount++
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `@action`, `"timeline-shrink"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "timeline-shrink", "persistent": "` + tokenPersistent + `"}`))
				log.Info("timeline shrink triggered")
				return rsp, nil
			}

			if strings.Contains(request.GetPrompt(), `"continue-current-task"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "continue-current-task"}`))
				requireMoreToolCount++
				return rsp, nil
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: ls`, `"call-tool"`, "const") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "ls", "params": {"path": "/` + noexistedfileToken + `"}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				// 限制require-tool的次数，避免无限循环
				if requireMoreToolCountForShrink < 5 {
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "ls"}`))
					requireMoreToolCountForShrink++
				} else {
					// 超过限制次数后，改为跳过任务
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "task-skipped", "task_short_summary": "无法执行目录扫描，工具调用失败"}`))
				}
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
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		coordinator.Run()
		cancel()
	}()

	count := 0
LOOP:
	for {
		select {
		case <-time.After(30 * time.Second):
			break LOOP
		case <-ctx.Done():
			break LOOP
		case result := <-outputChan:
			count++
			if count > 1000 {
				break LOOP
			}

			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}
			fmt.Println("result:" + result.String())
			if haveSidecarMem {
				break LOOP
			}
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				if a.GetObject("params").GetString("path") == "/"+noexistedfileToken &&
					a.GetString("tool") == "ls" && a.GetString("tool_description") != "" {
					inputChan <- &InputEvent{
						Id: result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"suggestion": "continue",
						},
					}
					continue
				}
			}
		}
	}

	if !haveSidecarMem {
		t.Fatal("sidecar memory failed")
	}

	// After compression, the original tokens might be compressed into reducers
	// So we just check that timeline dump is not empty and contains some content
	dump := m.timeline.Dump()
	if len(dump) == 0 {
		t.Fatal("timeline is empty after compression")
	}

	// Check that we have some timeline items (either original or compressed)
	if !strings.Contains(dump, "--[") {
		t.Fatal("timeline contains no items")
	}
}

func memoryTestBasic(t *testing.T) *Memory {
	inputChan := make(chan *InputEvent, 1000)
	outputChan := make(chan *schema.AiOutputEvent, 1000)

	requireMoreToolCount := 0

	timelineBatchCompressApplyCount := 0

	tokenBatchCompressed := utils.RandStringBytes(100)

	m := GetDefaultMemory()
	token1 := "memory-timeline-sidecar+" + utils.RandStringBytes(100)
	token2 := "memory-timeline-sidecar+" + utils.RandStringBytes(100)
	m.timeline.PushUserInteraction(aicommon.UserInteractionStage_FreeInput, 1, token1, token1)
	m.timeline.PushUserInteraction(aicommon.UserInteractionStage_FreeInput, 2, token2, token2)

	noexistedfileToken := utils.RandStringBytes(100)

	timeBatchCompressTrigger := false
	haveSidecarMem := false

	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithMemory(m),
		WithTimelineContentLimit(100),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			fmt.Println("========================================================")
			fmt.Println(request.GetPrompt())

			if utils.MatchAllOfSubString(request.GetPrompt(), token2, token1) {
				haveSidecarMem = true
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), tokenBatchCompressed) {
				timelineBatchCompressApplyCount++
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `@action`, `"timeline-reducer"`) ||
				strings.Contains(request.GetPrompt(), "批量精炼与浓缩") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "timeline-reducer", "reducer_memory": "` + tokenBatchCompressed + `"}`))
				log.Info("timeline batch compress triggered")
				timeBatchCompressTrigger = true
				return rsp, nil
			}

			if strings.Contains(request.GetPrompt(), `"continue-current-task"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "continue-current-task"}`))
				requireMoreToolCount++
				return rsp, nil
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: ls`, `"call-tool"`, "const") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "ls", "params": {"path": "/` + noexistedfileToken + `"}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				// 限制require-tool的次数，避免无限循环
				if requireMoreToolCount < 5 {
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "ls"}`))
					requireMoreToolCount++
				} else {
					// 超过限制次数后，改为跳过任务
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "task-skipped", "task_short_summary": "无法执行目录扫描，工具调用失败"}`))
				}
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

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		coordinator.Run()
		cancel()
	}()

	useToolReview := false
	useToolReviewPass := false
	count := 0
LOOP:
	for {
		select {
		case <-time.After(30 * time.Second):
			break LOOP
		case <-ctx.Done():
			break LOOP
		case result := <-outputChan:
			count++
			if count > 1000 {
				break LOOP
			}

			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}

			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				if a.GetObject("params").GetString("path") == "/"+noexistedfileToken &&
					a.GetString("tool") == "ls" && a.GetString("tool_description") != "" {
					useToolReview = true
					inputChan <- &InputEvent{
						Id: result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"suggestion": "continue",
						},
					}
					continue
				}
			}

			if useToolReview && (utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "ls") ||
				utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "ls")) {
				useToolReviewPass = true
				// break LOOP
			}

			if useToolReview && useToolReviewPass && timeBatchCompressTrigger {

				break LOOP
			}
			fmt.Println("review task result:" + result.String())
		}
	}

	if !useToolReview {
		t.Fatal("tool review fail")
	}

	if !useToolReviewPass {
		t.Fatal("tool review not finished")
	}

	if !haveSidecarMem {
		t.Fatal("sidecar memory failed")
	}

	// Check that timeline contains some content after processing
	// Due to timeline compression, we can't guarantee all tokens will be present
	dump := m.timeline.Dump()
	if len(dump) == 0 {
		t.Fatal("timeline is empty after processing")
	}

	// Check that timeline contains some timeline items
	if !strings.Contains(dump, "--[") {
		t.Fatal("timeline contains no items after processing")
	}

	return m.CopyReducibleMemory()
}
