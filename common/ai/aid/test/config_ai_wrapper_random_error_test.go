package test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	aicommon.RegisterDefaultAIRuntimeInvoker(aireact.BuildReActInvoker)
}

func Map2Json(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func ContinueSuggestionInputEvent(id string) *ypb.AIInputEvent {
	return &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        id,
		InteractiveJSONInput: Map2Json(map[string]any{
			"suggestion": "continue",
		}),
	}
}

func SuggestionInputEvent(id string, suggestion string, extra string) *ypb.AIInputEvent {
	return &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        id,
		InteractiveJSONInput: Map2Json(map[string]any{
			"suggestion":   suggestion,
			"extra_prompt": extra,
		}),
	}
}

func SuggestionInputEventEx(id string, params map[string]any) *ypb.AIInputEvent {
	return &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        id,
		InteractiveJSONInput: Map2Json(params),
	}
}

func SyncInputEvent(syncType string) *ypb.AIInputEvent {
	return &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      syncType,
	}
}

func SyncInputEventEx(syncType string, SyncID string) *ypb.AIInputEvent {
	return &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      syncType,
		SyncID:        SyncID,
	}
}

func TestCoordinator_RandomAICallbackError(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)

	m := new(sync.Mutex)
	var errLimit int64 = 2
	var count = new(int64)

	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(3),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			m.Lock()
			defer m.Unlock()

			countInt64 := atomic.AddInt64(count, 1)
			if countInt64 <= errLimit {
				return nil, utils.Errorf("mock, unknown err[%v]", count)
			}

			count = new(int64)

			if rsp, err := tryHandleNewPlanFlowPrompt(config, prompt, defaultTestPlanFromDocJSON); rsp != nil {
				return rsp, err
			}

			rsp := aicommon.NewAIResponse(config)
			defer rsp.Close()

			if utils.MatchAllOfSubString(prompt, "capability matcher", "matched_identifiers") ||
				utils.MatchAllOfSubString(prompt, `"const": "capability-catalog-match"`, "matched_identifiers") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "capability-catalog-match", "matched_identifiers": []}`))
				return rsp, nil
			}

			if isTaskSummaryPrompt(prompt) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "ok", "task_short_summary": "ok", "task_long_summary": "ok"}`))
				return rsp, nil
			}

			if strings.Contains(prompt, "tag-selection") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "tag-selection", "tags": ["test"]}`))
				return rsp, nil
			}

			if strings.Contains(prompt, "memory-triage") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "memory-triage", "memory_entities": []}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "ok"}`))
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
		ins.Run()
	}()

	parsedTask := false
	consumptionCheck := false
	pingPongCheck := false
	syncTaskCheck := false
LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				// 解析JSON数据
				var data = map[string]any{}
				err := json.Unmarshal([]byte(result.Content), &data)
				if err != nil {
					t.Fatal(err)
				}

				// 检查是否包含预期的任务描述
				if plansRaw, ok := data["plans"]; ok {
					plansJson, _ := json.Marshal(plansRaw)
					if strings.Contains(string(plansJson), `将最大文件的路径和大小以可读格式输出`) {
						parsedTask = true
						inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
						continue
					}
				}
			}
			if parsedTask && result.Type == schema.EVENT_TYPE_CONSUMPTION {
				var data = map[string]any{}
				err := json.Unmarshal([]byte(result.Content), &data)
				if err != nil {
					t.Fatal(err)
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
					inputChan.SafeFeed(SyncInputEvent(aicommon.SYNC_TYPE_PING))
					continue
				}
			}

			if consumptionCheck && result.Type == schema.EVENT_TYPE_PONG {
				pingPongCheck = true
				inputChan.SafeFeed(SyncInputEvent(aicommon.SYNC_TYPE_PLAN))
				continue
			}

			if pingPongCheck && result.Type == schema.EVENT_TYPE_PLAN {
				var i = make(aitool.InvokeParams, 0)
				if err := json.Unmarshal([]byte(result.Content), &i); err != nil {
					t.Fatal(err)
				}
				tasksRaw, ok := i.GetObject("root_task")["subtasks"]
				if !ok {
					t.Fatal("subtasks not found")
				}
				tasks := tasksRaw.([]any)
				var taskCount = len(tasks)
				if taskCount > 0 {
					syncTaskCheck = true
					break LOOP
				}
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
	if !consumptionCheck {
		t.Fatal("consumption check failed")
	}
	if !pingPongCheck {
		t.Fatal("pingPong check failed")
	}
	if !syncTaskCheck {
		t.Fatal("sync check failed")
	}
}
