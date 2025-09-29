package aid

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_Timeline_ToolUse_BatchCompression(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)

	requireMoreToolCount := 0

	timelineBatchCompressTrigger := false
	timelineBatchCompressApplyCount := 0

	tokenCompressed := utils.RandStringBytes(100)

	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithMaxTaskContinue(100),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithTimelineContentLimit(10),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			fmt.Println("========================================================")
			fmt.Println(request.GetPrompt())

			if utils.MatchAllOfSubString(request.GetPrompt(), tokenCompressed) {
				timelineBatchCompressApplyCount++
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `@action`, `"timeline-reducer"`) ||
				strings.Contains(request.GetPrompt(), "批量精炼与浓缩") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "timeline-reducer", "reducer_memory": "` + tokenCompressed + `"}`))
				log.Info("timeline batch compress triggered")
				timelineBatchCompressTrigger = true
				return rsp, nil
			}

			if strings.Contains(request.GetPrompt(), `"continue-current-task"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "continue-current-task"}`))
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

			if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "ls") {
				useToolReviewPass = true
			}

			if useToolReviewPass && timelineBatchCompressTrigger && requireMoreToolCount > 3 {
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

	if requireMoreToolCount <= 3 {
		t.Fatal("require more tool count not proper")
	}

	if !timelineBatchCompressTrigger {
		t.Fatal("timeline batch compress not triggered")
	}

	if timelineBatchCompressApplyCount <= 0 {
		t.Fatal("timeline batch compress count not proper")
	}
}
