package aid

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
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
	m.timeline.PushUserInteraction(UserInteractionStage_FreeInput, 1, token1, token1)
	m.timeline.PushUserInteraction(UserInteractionStage_FreeInput, 2, token2, token2)
	require.Contains(t, m.timeline.Dump(), token1, token2)

	fmt.Println(m.timeline.Dump())

	noexistedfileToken := utils.RandStringBytes(100)
	haveSidecarMem := false
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithMemory(m),
		WithTimeLineLimit(3),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
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
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "ls"}`))
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

	count := 0
LOOP:
	for {
		select {
		case <-time.After(30 * time.Second):
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

	if !utils.MatchAllOfSubString(m.timeline.Dump(), token1, token2) {
		t.Fatal("timeline not right")
	}
}

func memoryTestBasic(t *testing.T) *Memory {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)

	requireMoreToolCount := 0

	timelienShrinkApplyCount := 0

	tokenPersistent := utils.RandStringBytes(100)

	m := GetDefaultMemory()
	token1 := "memory-timeline-sidecar+" + utils.RandStringBytes(100)
	token2 := "memory-timeline-sidecar+" + utils.RandStringBytes(100)
	m.timeline.PushUserInteraction(UserInteractionStage_FreeInput, 1, token1, token1)
	m.timeline.PushUserInteraction(UserInteractionStage_FreeInput, 2, token2, token2)

	noexistedfileToken := utils.RandStringBytes(100)

	timeshrinkTrigger := false
	haveSidecarMem := false

	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithMemory(m),
		WithTimeLineLimit(3),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			fmt.Println("========================================================")
			fmt.Println(request.GetPrompt())

			if utils.MatchAllOfSubString(request.GetPrompt(), token2, token1) {
				haveSidecarMem = true
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), tokenPersistent) {
				timelienShrinkApplyCount++
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `@action`, `"timeline-shrink"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "timeline-shrink", "persistent": "` + tokenPersistent + `"}`))
				log.Info("timeline shrink triggered")
				timeshrinkTrigger = true
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
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "ls"}`))
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

	useToolReview := false
	useToolReviewPass := false
	count := 0
LOOP:
	for {
		select {
		case <-time.After(30 * time.Second):
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

			if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to execute tool:", "ls") {
				useToolReviewPass = true
				// break LOOP
			}

			if useToolReview && useToolReviewPass && timeshrinkTrigger {
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

	if !utils.MatchAllOfSubString(m.timeline.Dump(), token1, token2, noexistedfileToken) {
		t.Fatal("timeline not right")
	}
	return m.CopyReducibleMemory()
}
