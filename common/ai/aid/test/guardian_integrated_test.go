package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedToolCalling(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string, params string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	rsp := i.NewAIResponse()
	defer rsp.Close()
	fmt.Println("===========" + "request:" + "===========\n" + req.GetPrompt())
	if utils.MatchAllOfSubString(prompt, "plan: when user needs to create or refine a plan for a specific task, if need to search") {

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
	}

	if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {

		rsp.EmitOutputStream(bytes.NewBufferString(params))
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "short_summary") {
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "summary","task_short_summary":"mock"}`))
		time.Sleep(2 * time.Second)
		return rsp, nil
	}

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestCoordinator_GUARDIAN_OUTPUT_SMOKING_ToolUseReview(t *testing.T) {
	riskControlCalled := false
	guardianSmokingTestPassed := false
	var id []string

	mockRiskControl := func(ctx context.Context, config *aicommon.Config, ep *aicommon.Endpoint) (*aicommon.Action, error) {
		if !riskControlCalled {
			riskControlCalled = true
		}

		materials := ep.GetReviewMaterials()

		data := materials.GetString("id")
		if len(data) > 3 {
			id = append(id, data)
		}

		fmt.Printf("%v", materials)
		p := make(aitool.InvokeParams)
		p["risk_score"] = 0.2 // low risk score, quickly pass
		p["reason"] = "test reason"
		return aicommon.NewSimpleAction("object", p), nil
	}

	token2 := "status-..." + utils.RandStringBytes(20)

	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAiAgreeRiskControl(mockRiskControl),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "ls", `{"@action": "call-tool", "tool": "ls", "params": {"path": "/abc-target"}}`)
		}),
		aicommon.WithAIAgree(),
		aicommon.WithGuardianEventTrigger(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, func(event *schema.AiOutputEvent, emitter aicommon.GuardianEmitter, caller aicommon.AICaller) {
			if event.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				guardianSmokingTestPassed = true
			} else {
				guardianSmokingTestPassed = false
			}
			emitter.EmitStatus(token2, token2)
		}),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	useToolReview := false
	useToolReviewPass := false
	count := 0
	riskControlMsg := 0
	guardianOutputPass := false
LOOP:
	for {
		select {
		case <-time.After(30 * time.Second):
			break LOOP
		case result := <-outputChan:
			count++
			if count > 400 {
				break LOOP
			}
			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				if a.GetObject("params").GetString("path") == "/abc-target" &&
					a.GetString("tool") == "ls" && a.GetString("tool_description") != "" {
					useToolReview = true
					continue
				}
			}

			if result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
				fmt.Println("result:" + result.String())
			}

			if result.Type == schema.EVENT_TYPE_AI_REVIEW_COUNTDOWN {
				riskControlMsg++
				continue
			}

			if strings.Contains(result.String(), token2) {
				guardianOutputPass = true
				continue
			}

			if utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:") {
				if useToolReview && riskControlMsg >= 2 && guardianOutputPass {
					useToolReviewPass = true
					break LOOP
				}
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

	if !guardianSmokingTestPassed {
		t.Fatal("guardian smoking test failed")
	}

	if !guardianOutputPass {
		t.Fatal("guardian output test failed")
	}
}

func TestCoordinator_GUARDIAN_SMOKING_ToolUseReview(t *testing.T) {
	riskControlCalled := false
	guardianSmokingTestPassed := false
	var id []string
	mockRiskControl := func(ctx context.Context, config *aicommon.Config, ep *aicommon.Endpoint) (*aicommon.Action, error) {
		if !riskControlCalled {
			riskControlCalled = true
		}

		materials := ep.GetReviewMaterials()

		data := materials.GetString("id")
		if len(data) > 3 {
			id = append(id, data)
		}

		fmt.Printf("%v", materials)
		p := make(aitool.InvokeParams)
		p["risk_score"] = 0.2 // low risk score, quickly pass
		p["reason"] = "test reason"
		return aicommon.NewSimpleAction("object", p), nil
	}

	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAiAgreeRiskControl(mockRiskControl),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "ls", `{"@action": "call-tool", "tool": "ls", "params": {"path": "/abc-target"}}`)
		}),
		aicommon.WithAIAgree(),
		aicommon.WithGuardianEventTrigger(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, func(event *schema.AiOutputEvent, emitter aicommon.GuardianEmitter, caller aicommon.AICaller) {
			if event.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				guardianSmokingTestPassed = true
			} else {
				guardianSmokingTestPassed = false
			}
		}),
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
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				if a.GetObject("params").GetString("path") == "/abc-target" &&
					a.GetString("tool") == "ls" && a.GetString("tool_description") != "" {
					useToolReview = true
					continue
				}
			}

			if result.Type == schema.EVENT_TYPE_AI_REVIEW_COUNTDOWN {
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

	if !guardianSmokingTestPassed {
		t.Fatal("guardian smoking test failed")
	}
}

func TestCoordinator_GUARDIAN_StreamSmocking_ToolUseReview(t *testing.T) {
	riskControlCalled := false
	guardianSmokingTestPassed := false
	var id []string
	var allData []any
	mockRiskControl := func(ctx context.Context, config *aicommon.Config, ep *aicommon.Endpoint) (*aicommon.Action, error) {
		if !riskControlCalled {
			riskControlCalled = true
		}

		materials := ep.GetReviewMaterials()

		data := materials.GetString("id")
		if len(data) > 3 {
			id = append(id, data)
			allData = append(allData, materials)
		}

		fmt.Printf("%v", materials)
		p := make(aitool.InvokeParams)
		p["risk_score"] = 0.2 // low risk score, quickly pass
		p["reason"] = "test reason"
		return aicommon.NewSimpleAction("object", p), nil
	}

	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAiAgreeRiskControl(mockRiskControl),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "ls", `{"@action": "call-tool", "tool": "ls", "params": {"path": "/abc-target"}}`)
		}),
		aicommon.WithAIAgree(),
		aicommon.WithGuardianMirrorStreamMirror("test", func(unlimitedChan *chanx.UnlimitedChan[*schema.AiOutputEvent], emitter aicommon.GuardianEmitter) {
			for event := range unlimitedChan.OutputChannel() {
				if event.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
					guardianSmokingTestPassed = true
				}
			}
		}),
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
			if count > 1000 {
				break LOOP
			}
			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				if a.GetObject("params").GetString("path") == "/abc-target" &&
					a.GetString("tool") == "ls" && a.GetString("tool_description") != "" {
					useToolReview = true
					continue
				}
			}

			if result.Type == schema.EVENT_TYPE_AI_REVIEW_COUNTDOWN {
				riskControlMsg++
				continue
			}

			if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "ls") && riskControlMsg >= 2 {
				useToolReviewPass = true
				break LOOP
			}
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
	require.Equal(t, len(id), 2, fmt.Sprintf("%v", allData))

	if !guardianSmokingTestPassed {
		t.Fatal("guardian smoking test failed")
	}
}
