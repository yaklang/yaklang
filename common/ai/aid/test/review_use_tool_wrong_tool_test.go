package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
)

func TestCoordinator_ToolUseReview_WrongTool_SuggestionTools(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 10)

	lsReviewed := false
	nowReviewed := false
	toolName1 := "ls"
	toolName2 := "now"

	coordinator, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			i := config

			prompt := request.GetPrompt()

			if strings.Contains(prompt, "意图识别与上下文增强系统") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finalize_enrichment", "intent_summary": "mocked intent analysis", "recommended_capabilities": "", "context_notes": ""}`))
				rsp.Close()
				return rsp, nil
			}

			if strings.Contains(prompt, "数据处理和总结提示小助手") {
				rsp := i.NewAIResponse()
				if strings.Contains(prompt, "tag-selection") {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "tag-selection", "tags": ["test"]}`))
				} else if strings.Contains(prompt, "memory-triage") {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "memory-triage", "memory_entities": []}`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object"}`))
				}
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "plan: when user needs to create or refine a plan for a specific task, if need to search") {
				rsp := i.NewAIResponse()
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
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName1 + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
				rsp.Close()
				return rsp, nil

			}

			if utils.MatchAllOfSubString(prompt, "require-tool", "abandon") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "require-tool", "tool": ` + toolName2 + `}`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "input" : "mocked-echo-params" }}`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "short_summary") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "summary","task_short_summary":"mock"}`))
				rsp.Close()
				return rsp, nil
			}

			fmt.Println("Unexpected prompt:", prompt)

			return nil, utils.Errorf("unexpected prompt: %s", prompt)

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
			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				toolname := a.GetString("tool")
				if toolname == "ls" {
					lsReviewed = true
					useToolReview = true
					inputChan.SafeFeed(SuggestionInputEventEx(result.GetInteractiveId(), map[string]any{
						"suggestion":      "wrong_tool",
						"suggestion_tool": "tree,now",
					}))
				} else if toolname == "now" {
					nowReviewed = true
					inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				}
			}

			if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "now") {
				useToolReviewPass = true
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

	if !lsReviewed {
		t.Fatal("ls tool review not finished")
	}

	if !nowReviewed {
		t.Fatal("now tool review not finished")
	}
}
