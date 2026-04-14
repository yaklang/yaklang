package test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedClarification(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, flag string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	fmt.Println("===========" + "request:" + "===========\n" + req.GetPrompt())

	if rsp, err := tryHandleNewPlanFlowPrompt(i, prompt, mockedToolCallingPlanJSON); rsp != nil {
		return rsp, err
	}

	if isIntentEnrichmentPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finalize_enrichment", "intent_summary": "mocked intent analysis", "recommended_capabilities": "", "context_notes": ""}`))
		rsp.Close()
		return rsp, nil
	}

	if isMemorySummaryPrompt(prompt) {
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

	if isNextActionDecisionPrompt(prompt) && strings.Contains(prompt, "ask_for_clarification") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "ask_for_clarification", "ask_for_clarification_payload": {"question": "...mocked question...", "options": ["` + flag + `", "option2", "option3"]} },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if isSummaryPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "summary","task_short_summary":"mock"}`))
		rsp.Close()
		return rsp, nil
	}

	return nil, unexpectedPromptError(prompt)
}

func TestCoodinator_AllowRequireForUserInteract_UserAct(t *testing.T) {
	// 首先从 coordinator 开始
	// 测试需要尊崇如下几个要点，任务随便是啥都行，只执行第一个工具为止
	// 第一个工具 require 为 require-user-interact，
	// 这个工具比较特殊：无法用户审核，因为它本身就包含了用户交互
	token1 := utils.RandStringBytes(200)

	mockCallback := func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return mockedClarification(i, r, token1)
	}

	token2 := utils.RandStringBytes(200)
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	checkToken1 := false
	checkToken2 := false
	interactiveCheck := false
	coordinator, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(mockCallback),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	timeoutDurationSecond := time.Duration(60) * time.Second
	if utils.InGithubActions() {
		timeoutDurationSecond = time.Duration(10) * time.Second
	}
	count := 0
LOOP:
	for {
		select {
		case <-time.After(timeoutDurationSecond):
			break LOOP
		case result := <-outputChan:
			count++
			if count > 500 {
				break LOOP
			}
			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}

			if interactiveCheck {
				if !checkToken2 {
					if strings.Contains(result.String(), token2) {
						if result.Type == schema.EVENT_TYPE_REVIEW_RELEASE {
							checkToken2 = true
							break LOOP
						}
					}
				}
				fmt.Println("result:" + result.String())
				continue
			}

			if checkToken1 && !interactiveCheck {
				if result.Type == schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
					interactiveCheck = true
					inputChan.SafeFeed(SuggestionInputEvent(result.GetInteractiveId(), "continue", "你好"+token2))
					continue
				}
			}

			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				t.Fatal("no tool review required")
			}

			if !checkToken1 && strings.Contains(result.String(), token1) {
				checkToken1 = true
				continue
			}

		}
	}

	assert.True(t, interactiveCheck, "interactive check failed")
	assert.True(t, checkToken2, "token2 check failed")
}
