package aid

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
)

func TestCoordinator_PIMatrix_ToolUseReview_NEG(t *testing.T) {
	riskControlForgeName := utils.RandStringBytes(10)
	riskControlCalled := false
	var id []string
	err := RegisterAIDBuildinForge(riskControlForgeName, func(c context.Context, params []*ypb.ExecParamItem, opts ...Option) (*aicommon.Action, error) {
		if !riskControlCalled {
			riskControlCalled = true
		}

		for _, i := range params {
			if i.Key == "id" && len(i.Value) > 3 {
				id = append(id, i.Value)
			}
		}

		spew.Dump(params)
		p := make(aitool.InvokeParams)
		p["probability"] = 0.5
		p["impact"] = 0.5
		p["reason"] = "test reason"
		return aicommon.NewSimpleAction("", p), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithRiskControlForgeName(utils.RandStringBytes(100), nil), // bad not right risk control
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
		WithAIAgree(),
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
			if count > 100 {
				break LOOP
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

	if riskControlCalled {
		t.Fatal("risk control called(not right in this negative testcase)")
	}

	id = utils.RemoveRepeatedWithStringSlice(id)
	require.Equal(t, len(id), 0)
}

func TestCoordinator_PIMatrix_ToolUseReview(t *testing.T) {
	riskControlForgeName := utils.RandStringBytes(10)
	riskControlCalled := false
	var id []string
	err := RegisterAIDBuildinForge(riskControlForgeName, func(c context.Context, params []*ypb.ExecParamItem, opts ...Option) (*aicommon.Action, error) {
		if !riskControlCalled {
			riskControlCalled = true
		}

		for _, i := range params {
			if i.Key == "id" && len(i.Value) > 3 {
				id = append(id, i.Value)
			}
		}

		spew.Dump(params)
		p := make(aitool.InvokeParams)
		p["probability"] = 0.5
		p["impact"] = 0.5
		p["reason"] = "test reason"
		return aicommon.NewSimpleAction("", p), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithRiskControlForgeName(riskControlForgeName, nil),
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
		WithAIAgree(),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	useToolReview := false
	useToolReviewPass := false
	count := 0
	riskControlMsg := 0
LOOP:
	for {
		select {
		case <-time.After(30 * time.Second):
			break LOOP
		case result := <-outputChan:
			count++
			if count > 100 {
				break LOOP
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

			if result.Type == schema.EVENT_TYPE_RISK_CONTROL_PROMPT {
				riskControlMsg++
				continue
			}

			if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "ls") && riskControlMsg >= 2 {
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

	if !riskControlCalled {
		t.Fatal("risk control not called")
	}

	id = utils.RemoveRepeatedWithStringSlice(id)
	require.Equal(t, len(id), 2)
}

func TestPIM_Basic(t *testing.T) {
	UnregisterAIDBuildinForge("pimatrix-mock")
	err := RegisterAIDBuildinForge("pimatrix-mock", func(c context.Context, params []*ypb.ExecParamItem, opts ...Option) (*aicommon.Action, error) {
		a := aicommon.NewSimpleAction("", nil)
		a.GetParams().Set("probability", 0.5)
		a.GetParams().Set("impact", 0.5)
		return a, nil
	})
	if err != nil {
		t.Fatal(err)
		return
	}

	a, err := ExecuteAIForge(context.Background(), "pimatrix-mock", []*ypb.ExecParamItem{
		{Key: "a", Value: "b"},
	})
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(a)
	require.True(t, a.GetFloat("probability") == 0.5)
	require.True(t, a.GetFloat("impact") == 0.5)
}
