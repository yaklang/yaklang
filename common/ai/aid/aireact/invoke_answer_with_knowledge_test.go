package aireact

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Mock AI callback for answer with knowledge flow
func newMockedAnswerWithKnowledgeFlow(token string) aicommon.AICallbackType {
	callback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		if utils.MatchAllOfSubString(prompt, string(ActionDirectlyAnswer), string(ActionRequireTool), string(ActionKnowledgeEnhanceAnswer)) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "knowledge_enhance_answer", "answer_payload": "Go is statically typed.", "knowledge_payload": "Go typing" },
"human_readable_thought": "Using knowledge to answer.", "cumulative_summary": "Summary with knowledge."}
`))
			rsp.Close()
			return rsp, nil
		}
		// Add: handle the AI call after knowledge is enhanced
		if utils.MatchAllOfSubString(prompt, "MUST use 'directly_answer'") {
			if !utils.MatchAllOfSubString(prompt, token) {
				return nil, utils.Errorf("knowledge token should not appear in the final answer prompt")
			}
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object",
 "next_action": {
   "type": "directly_answer",
   "answer_payload": "Go is a statically typed, compiled programming language designed at Google. It supports concurrency via goroutines and channels.",
   "cumulative_summary": "User asked about Go. Provided enhanced answer including concurrency features."
 },
 "human_readable_thought": "Final answer after knowledge enhancement.",
 "cumulative_summary": "Final summary after knowledge enhancement."
}
`))
			rsp.Close()
			return rsp, nil
		}
		if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "User is satisfied with the answer."}`))
			rsp.Close()
			return rsp, nil
		}
		return nil, utils.Errorf("unexpected prompt: %s", prompt)
	}

	return callback
}

func TestReAct_AnswerWithKnowledge_FullFlow(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	handle, knowledgeToken := NewMockEnhanceHandlerAndToken()
	_, err := NewReAct(
		WithAICallback(newMockedAnswerWithKnowledgeFlow(knowledgeToken)),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		WithKnowledgeEnhanceHandle(handle),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "What is Go?",
		}
	}()

	after := time.After(10 * time.Second)
	var gotResult, gotKnowledge, gotSatisfied bool

LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_KNOWLEDGE) && utils.MatchAllOfSubString(e.Content, knowledgeToken) {
				gotKnowledge = true
			}
			if e.Type == string(schema.EVENT_TYPE_RESULT) {
				gotResult = true
			}
			if e.Type == string(schema.EVENT_TYPE_STRUCTURED) {
				if utils.MatchAllOfSubString(e.Content, string(TaskStatus_Completed)) {
					gotSatisfied = true
				}
			}
			if gotResult && gotKnowledge && gotSatisfied {
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !gotKnowledge {
		t.Fatal("Expected knowledge event")
	}
	if !gotResult {
		t.Fatal("Expected result event")
	}
	if !gotSatisfied {
		t.Fatal("Expected satisfaction event")
	}
}

func newMockedAnswerWithKnowledgeUnsatisfied(token, okToken string) aicommon.AICallbackType {
	satisfiedToken := uuid.NewString()
	callback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		if utils.MatchAllOfSubString(prompt, "directly_answer", "knowledge_enhance") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "knowledge_enhance_answer", "answer_payload": "Go is statically typed.", "knowledge_payload": "Go typing" },
"human_readable_thought": "Using knowledge to answer.", "cumulative_summary": "Summary with knowledge."}
`))
			rsp.Close()
			return rsp, nil
		}
		// Add: handle the AI call after knowledge is enhanced
		if utils.MatchAllOfSubString(prompt, "MUST use 'directly_answer'") {
			rsp := i.NewAIResponse()
			if !utils.MatchAllOfSubString(prompt, token) {
				return nil, utils.Errorf("knowledge token should not appear in the final answer prompt")
			}

			if utils.MatchAllOfSubString(prompt, okToken) {
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object",
 "next_action": {
   "type": "directly_answer",
   "answer_payload": "%s",
   "cumulative_summary": "User asked about Go. Provided enhanced answer, but user wants more details."
 },
 "human_readable_thought": "Final answer after knowledge enhancement, but user not satisfied.",
 "cumulative_summary": "Final summary, user wants more."
}
`, satisfiedToken)))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object",
 "next_action": {
   "type": "directly_answer",
   "answer_payload": "Go is a statically typed, compiled programming language designed at Google. It supports concurrency via goroutines and channels.",
   "cumulative_summary": "User asked about Go. Provided enhanced answer including concurrency features."
 },
 "human_readable_thought": "Final answer after knowledge enhancement.",
 "cumulative_summary": "Final summary after knowledge enhancement."
}
`))
			}
			rsp.Close()
			return rsp, nil
		}
		if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied") {
			rsp := i.NewAIResponse()
			if utils.MatchAllOfSubString(prompt, satisfiedToken) {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "User is satisfied with the answer."}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "User wants more details."}`))
			}
			rsp.Close()
			return rsp, nil
		}
		return nil, utils.Errorf("unexpected prompt: %s", prompt)
	}

	return callback
}

func satisfiedMockHandle(token, okToken string) func(ctx context.Context, query string) (<-chan aicommon.EnhanceKnowledge, error) {
	checkData1 := aicommon.NewBasicEnhanceKnowledge(
		token,
		"mock",
		0.82,
	)

	checkData2 := aicommon.NewBasicEnhanceKnowledge(
		okToken,
		"mock",
		0.82,
	)

	first := true

	return func(ctx context.Context, query string) (<-chan aicommon.EnhanceKnowledge, error) {
		if first {
			first = false
			result := chanx.NewUnlimitedChan[aicommon.EnhanceKnowledge](ctx, 10)
			go func() {
				defer result.Close()
				for _, k := range []aicommon.EnhanceKnowledge{checkData1} {
					result.SafeFeed(k)
				}
			}()
			return result.OutputChannel(), nil
		}

		result := chanx.NewUnlimitedChan[aicommon.EnhanceKnowledge](ctx, 10)
		go func() {
			defer result.Close()
			for _, k := range []aicommon.EnhanceKnowledge{checkData2} {
				result.SafeFeed(k)
			}
		}()
		return result.OutputChannel(), nil

	}
}

// Test satisfaction loop: user not satisfied, triggers another iteration

func TestReAct_AnswerWithKnowledge_SatisfactionLoop(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	firstToken := uuid.NewString()
	okToken := uuid.NewString()

	_, err := NewReAct(
		WithAICallback(newMockedAnswerWithKnowledgeUnsatisfied(firstToken, okToken)),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		WithMaxIterations(2),
		WithKnowledgeEnhanceHandle(satisfiedMockHandle(firstToken, okToken)),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "What is Go?",
		}
	}()

	after := time.After(5 * time.Second)
	var gotKnowledge1, gotKnowledge2, gotResult, gotSatisfied bool

LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_KNOWLEDGE) {
				if utils.MatchAllOfSubString(e.Content, firstToken) {
					gotKnowledge1 = true
				}
				if gotKnowledge1 && utils.MatchAllOfSubString(e.Content, okToken) {
					gotKnowledge2 = true
				}
			}
			if e.Type == string(schema.EVENT_TYPE_RESULT) {
				gotResult = true
			}
			if e.Type == string(schema.EVENT_TYPE_STRUCTURED) {
				if utils.MatchAllOfSubString(e.Content, string(TaskStatus_Completed)) {
					gotSatisfied = true
				}
			}
			if gotKnowledge1 && gotKnowledge2 && gotResult && gotSatisfied {
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !gotKnowledge1 || !gotKnowledge2 {
		t.Fatal("Expected knowledge event")
	}
	if !gotResult {
		t.Fatal("Expected result")
	}
	if !gotSatisfied {
		t.Fatal("Expected satisfied event")
	}
}
