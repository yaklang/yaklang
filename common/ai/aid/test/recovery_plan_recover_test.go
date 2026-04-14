package test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func recoverPlan(t *testing.T, uuid string) {
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	planJSON := `{
    "@action": "plan_from_document",
    "main_task": "在指定目录中找到最大的文件",
    "main_task_goal": "明确 /Users/v1ll4n/Projects/yaklang 目录下哪个文件占用空间最大，并输出该文件的路径和大小",
    "tasks": [
        {"subtask_name": "遍历目标目录", "subtask_goal": "递归扫描 /Users/v1ll4n/Projects/yaklang 目录，获取所有文件的路径和大小"},
        {"subtask_name": "筛选最大文件", "subtask_goal": "根据文件大小比较，确定目录中占用空间最大的文件"},
        {"subtask_name": "输出结果", "subtask_goal": "将最大文件的路径和大小以可读格式输出"}
    ]
}`
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	defer func() {
		inputChan.Close()
		go func() {
			for range outputChan {
			}
		}()
		time.Sleep(100 * time.Millisecond)
		close(outputChan)
	}()
	recoverCtx, recoverCancel := context.WithCancel(context.Background())
	defer recoverCancel()
	ord, err := aid.NewFastRecoverCoordinatorContext(
		recoverCtx,
		uuid,
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			if rsp, err := tryHandleNewPlanFlowPrompt(config, prompt, planJSON); rsp != nil {
				return rsp, err
			}
			rsp := config.NewAIResponse()
			defer rsp.Close()
			if isNextActionDecisionPrompt(prompt) {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "object",
    "next_action": {"type": "finish", "answer_payload": "task done"},
    "cumulative_summary": "done",
    "human_readable_thought": "done"
}`))
				return rsp, nil
			}
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "ok"}`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered from panic in ord.Run(): %v", r)
			}
		}()
		if err := ord.Run(); err != nil {
			log.Errorf("ord.Run() error: %v", err)
		}
	}()

	parsedTask := false
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()

LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				break LOOP
			}
		case <-timeout.C:
			t.Fatal("timeout waiting for plan review require event")
		case <-recoverCtx.Done():
			t.Fatal("context cancelled")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
}

func TestCoordinator_RecoverCase(t *testing.T) {
	planJSON := `{
    "@action": "plan_from_document",
    "main_task": "在指定目录中找到最大的文件",
    "main_task_goal": "明确 /Users/v1ll4n/Projects/yaklang 目录下哪个文件占用空间最大，并输出该文件的路径和大小",
    "tasks": [
        {"subtask_name": "遍历目标目录", "subtask_goal": "递归扫描 /Users/v1ll4n/Projects/yaklang 目录，获取所有文件的路径和大小"},
        {"subtask_name": "筛选最大文件", "subtask_goal": "根据文件大小比较，确定目录中占用空间最大的文件"},
        {"subtask_name": "输出结果", "subtask_goal": "将最大文件的路径和大小以可读格式输出"}
    ]
}`
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	defer func() {
		inputChan.Close()
		go func() {
			for range outputChan {
			}
		}()
		time.Sleep(100 * time.Millisecond)
		close(outputChan)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			if rsp, err := tryHandleNewPlanFlowPrompt(config, prompt, planJSON); rsp != nil {
				return rsp, err
			}
			rsp := config.NewAIResponse()
			defer rsp.Close()
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "ok"}`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered from panic in ins.Run(): %v", r)
			}
		}()
		if err := ins.Run(); err != nil {
			log.Errorf("ins.Run() error: %v", err)
		}
	}()

	parsedTask := false
	consumptionCheck := false
	mainTimeout := time.NewTimer(30 * time.Second)
	defer mainTimeout.Stop()

LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}
			if parsedTask && result.Type == schema.EVENT_TYPE_CONSUMPTION {
				var data = map[string]any{}
				err := json.Unmarshal([]byte(result.Content), &data)
				if err != nil {
					log.Errorf("failed to unmarshal consumption data: %v", err)
					continue
				}
				inputConsumption := int64(0)
				outputConsumption := int64(0)
				if i, ok := data["input_consumption"]; ok {
					inputConsumption = int64(utils.InterfaceToInt(i))
				}
				if o, ok := data["output_consumption"]; ok {
					outputConsumption = int64(utils.InterfaceToInt(o))
				}
				if inputConsumption > 0 && outputConsumption > 0 {
					consumptionCheck = true
					break LOOP
				}
			}
		case <-mainTimeout.C:
			t.Fatal("timeout waiting for test completion")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
	if !consumptionCheck {
		t.Fatal("consumption check failed")
	}

	recoverPlan(t, ins.Config.Id)
}
