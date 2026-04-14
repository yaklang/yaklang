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

func TestCoordinator_AICallSummaryEvent(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outChan := chanx.NewUnlimitedChan[*schema.AiOutputEvent](context.Background(), 100)

	ins, err := aid.NewCoordinator(
		"test-ai-call-summary",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outChan.SafeFeed(event)
		}),
		aid.WithPlanMocker(func(coordinator *aid.Coordinator) *aid.PlanResponse {
			return &aid.PlanResponse{
				RootTask: &aid.AiTask{
					Name: "test main task",
					Goal: "verify ai_call_summary event fields",
					Subtasks: []*aid.AiTask{
						{
							Name: "subtask-1",
							Goal: "test subtask goal",
						},
					},
				},
			}
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			rsp.SetModelInfo("test-provider", "test-model-v1")

			if isSummaryPrompt(prompt) {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "summary",
    "status_summary": "test status summary",
    "task_short_summary": "test short summary",
    "task_long_summary": "test long summary"
}`))
				time.Sleep(50 * time.Millisecond)
				rsp.Close()
				return rsp, nil
			}

			if isNextActionDecisionPrompt(prompt) {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "object",
    "next_action": {
        "type": "finish",
        "answer_payload": "test: task completed"
    },
    "cumulative_summary": "test cumulative summary",
    "human_readable_thought": "test: skip subtask"
}`))
				time.Sleep(50 * time.Millisecond)
				rsp.Close()
				return rsp, nil
			}

			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "test: default finish"}`))
			time.Sleep(50 * time.Millisecond)
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		ins.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	parsedTask := false
	summaryCheck := false
	pressureCheck := false
	firstByteCheck := false
	totalCostCheck := false
	outChannel := outChan.OutputChannel()

LOOP:
	for {
		select {
		case result := <-outChannel:
			fmt.Println("result:" + result.String())

			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE &&
				strings.Contains(result.String(), "verify ai_call_summary event fields") {
				parsedTask = true
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_PRESSURE {
				var data map[string]any
				if err := json.Unmarshal(result.Content, &data); err != nil {
					log.Errorf("failed to parse pressure event: %v", err)
					continue
				}
				requiredFields := []string{"current_cost_token_size", "pressure_token_size", "model_tier"}
				for _, field := range requiredFields {
					if _, ok := data[field]; !ok {
						t.Fatalf("pressure event missing '%s' field", field)
					}
				}
				pressureCheck = true
				continue
			}

			if result.Type == schema.EVENT_TYPE_AI_FIRST_BYTE_COST_MS {
				var data map[string]any
				if err := json.Unmarshal(result.Content, &data); err != nil {
					log.Errorf("failed to parse ai_first_byte_cost_ms: %v", err)
					continue
				}
				if _, ok := data["ms"]; !ok {
					t.Fatal("ai_first_byte_cost_ms missing 'ms' field")
				}
				if _, ok := data["model_name"]; !ok {
					t.Fatal("ai_first_byte_cost_ms missing 'model_name' field")
				}
				if _, ok := data["provider_name"]; !ok {
					t.Fatal("ai_first_byte_cost_ms missing 'provider_name' field")
				}
				if _, ok := data["model_tier"]; !ok {
					t.Fatal("ai_first_byte_cost_ms missing 'model_tier' field")
				}
				firstByteCheck = true
				log.Infof("ai_first_byte_cost_ms enriched fields verified: model_name=%v, provider_name=%v",
					data["model_name"], data["provider_name"])
				continue
			}

			if result.Type == schema.EVENT_TYPE_AI_TOTAL_COST_MS {
				var data map[string]any
				if err := json.Unmarshal(result.Content, &data); err != nil {
					log.Errorf("failed to parse ai_total_cost_ms: %v", err)
					continue
				}
				requiredFields := []string{"ms", "second", "model_name", "provider_name", "model_tier", "token_rate", "output_bytes", "output_duration_ms"}
				for _, field := range requiredFields {
					if _, ok := data[field]; !ok {
						t.Fatalf("ai_total_cost_ms missing '%s' field", field)
					}
				}
				totalCostCheck = true
				log.Infof("ai_total_cost_ms enriched fields verified: model=%v, provider=%v, token_rate=%v",
					data["model_name"], data["provider_name"], data["token_rate"])
				continue
			}

			if parsedTask && result.Type == schema.EVENT_TYPE_AI_CALL_SUMMARY {
				var data map[string]any
				if err := json.Unmarshal(result.Content, &data); err != nil {
					t.Fatalf("failed to parse ai_call_summary content: %v", err)
				}

				requiredFields := []string{
					"model_name", "provider_name", "model_tier",
					"first_byte_cost_ms", "total_cost_ms",
					"output_bytes", "estimated_output_tokens",
					"token_rate", "output_duration_ms",
					"input_token_size",
				}
				for _, field := range requiredFields {
					if _, ok := data[field]; !ok {
						t.Fatalf("ai_call_summary missing required field '%s', got: %v", field, data)
					}
				}

				totalCostMs := utils.InterfaceToFloat64(data["total_cost_ms"])
				if totalCostMs <= 0 {
					t.Fatalf("ai_call_summary total_cost_ms should be > 0, got: %v", totalCostMs)
				}

				inputTokenSize := utils.InterfaceToInt(data["input_token_size"])
				if inputTokenSize <= 0 {
					t.Fatalf("ai_call_summary input_token_size should be > 0, got: %v", inputTokenSize)
				}

				summaryCheck = true
				log.Infof("ai_call_summary verified: model=%v, provider=%v, total_cost_ms=%v, token_rate=%v, input_tokens=%v",
					data["model_name"], data["provider_name"],
					data["total_cost_ms"], data["token_rate"], data["input_token_size"])
				break LOOP
			}

		case <-time.After(3 * time.Minute):
			log.Errorf("test timeout: parsedTask=%t, pressureCheck=%t, summaryCheck=%t, firstByteCheck=%t, totalCostCheck=%t",
				parsedTask, pressureCheck, summaryCheck, firstByteCheck, totalCostCheck)
			t.Fatal("timeout waiting for ai_call_summary event")
		}
	}

	if !parsedTask {
		t.Fatal("plan review event not received")
	}
	if !pressureCheck {
		t.Fatal("pressure event missing required fields")
	}
	if !summaryCheck {
		t.Fatal("ai_call_summary event not received or fields incomplete")
	}
	if !firstByteCheck {
		t.Fatal("enriched ai_first_byte_cost_ms event not received")
	}
	if !totalCostCheck {
		t.Fatal("enriched ai_total_cost_ms event not received")
	}
}

func TestAICallSummary_ShouldNotSave(t *testing.T) {
	event := &schema.AiOutputEvent{
		Type: schema.EVENT_TYPE_AI_CALL_SUMMARY,
	}
	if event.ShouldSave() {
		t.Fatal("ai_call_summary events should not be persisted (ShouldSave must return false)")
	}
}
