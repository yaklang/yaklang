package aid

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_Timeline_ToolUse_TooMany_TimelineReducer(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)

	requireMoreToolCount := 0

	timelineShrinkTrigger := false
	timelineShrinkApplyCount := 0

	timelineReducerTrigger := false
	timelineReducerApplyCount := 0

	tokenPersistent := utils.RandStringBytes(100)
	tokenReducer := utils.RandStringBytes(100)

	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithTimeLineLimit(3),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfRegexp(request.GetPrompt(), tokenReducer) {
				timelineReducerApplyCount++
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), tokenPersistent) {
				timelineShrinkApplyCount++
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `@action`, `"timeline-reducer`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "timeline-reducer", "reducer_memory": "` + tokenReducer + `"}`))
				timelineReducerTrigger = true
				return rsp, nil
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `@action`, `"timeline-shrink"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "timeline-shrink", "persistent": "` + tokenPersistent + `"}`))
				log.Info("timeline shrink triggered")
				timelineShrinkTrigger = true
				return rsp, nil
			}

			fmt.Println("========================================================")
			fmt.Println(request.GetPrompt())

			if strings.Contains(request.GetPrompt(), `"require-more-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-more-tool"}`))
				requireMoreToolCount++
				if requireMoreToolCount > 10 {
					log.Info("requireMoreToolCount reached 10")
				}
				return rsp, nil
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: ls`, `"call-tool"`, "const") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "ls", "params": {"path": "/abc-target"}}`))
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

			if timelineReducerTrigger && timelineReducerApplyCount >= 1 {
				break LOOP
			}

			if result.Type == EVENT_TYPE_CONSUMPTION {
				continue
			}

			fmt.Println("result:" + result.String())
			if result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
				continue
			}

			if result.Type == EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				if a.GetObject("params").GetString("path") == "/abc-target" &&
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
			fmt.Println("review task result:" + result.String())
		}
	}

	if !useToolReview {
		t.Fatal("tool review fail")
	}

	if !useToolReviewPass {
		t.Fatal("tool review not finished")
	}

	if requireMoreToolCount <= 3 {
		t.Fatal("require more tool count not proper")
	}

	if !timelineShrinkTrigger {
		t.Fatal("timeline shrink not triggered")
	}

	if timelineShrinkApplyCount <= 3 {
		t.Fatal("timelien shrink count not proper")
	}

	if timelineReducerApplyCount < 1 {
		t.Fatal("timeline reducer not proper")
	}
}
