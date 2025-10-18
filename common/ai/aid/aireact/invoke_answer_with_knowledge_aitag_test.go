package aireact

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_AnswerWithKnowledge_FullFlow_AITAG_ANSWER(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	manager, token := aicommon.NewMockEKManagerAndToken()

	syncSignal := make(chan bool)

	ctx := utils.TimeoutContextSeconds(10)

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
		if utils.MatchAllOfSubString(prompt, "USE THIS FIELD ONLY IF @action is 'directly_answer'") {
			if !utils.MatchAllOfSubString(prompt, token) {
				return nil, utils.Errorf("knowledge token should not appear in the final answer prompt")
			}
			// extract aitag from prompt <|FINAL_ANSWER_(\w{4})|>
			// INSERT_YOUR_CODE
			// extract nonce from prompt using regex <|FINAL_ANSWER_(\w{4})|>
			var aitagNonce string
			nonceRe := regexp.MustCompile(`<\|FINAL_ANSWER_(\w{4})\|>`)
			matches := nonceRe.FindStringSubmatch(prompt)
			if len(matches) == 2 {
				aitagNonce = matches[1]
			} else {
				aitagNonce = "" // Not found
			}
			// You can use aitagNonce below if needed

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "directly_answer"}
<|FINAL_ANSWER_` + aitagNonce + `|>
Go is a statically typed, compiled programming language designed at Google. It supports concurrency via goroutines and channels.
<|FINAL_ANSWER_END_` + aitagNonce + `|>
`))
			rsp.Close()
			return rsp, nil
		}
		if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied") {
			in <- &ypb.AIInputEvent{ // 触发同步知识事件
				IsSyncMessage: true,
				SyncType:      SYNC_TYPE_KNOWLEDGE,
			}

			select {
			case <-syncSignal:
			case <-ctx.Done():
			}
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "User is satisfied with the answer."}`))
			rsp.Close()
			return rsp, nil
		}
		return nil, utils.Errorf("unexpected prompt: %s", prompt)
	}

	_, err := NewTestReAct(
		WithAICallback(callback),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		WithEnhanceKnowledgeManager(manager),
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

	var gotResult, gotKnowledge, gotSatisfied, gotSyncKnowledge bool

LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_KNOWLEDGE) && utils.MatchAllOfSubString(e.Content, token) {
				gotKnowledge = true
			}
			if e.Type == string(schema.EVENT_TYPE_RESULT) {
				if strings.Contains(e.String(), `Go is a statically typed, compiled programming language designed at Google. It supports concurrency via goroutines and channels.`) {
					gotResult = true
				}
			}

			if e.Type == string(schema.EVENT_TYPE_TASK_ABOUT_KNOWLEDGE) {
				if utils.MatchAllOfSubString(e.Content, token) {
					gotSyncKnowledge = true
					close(syncSignal)
				}
			}

			if e.Type == string(schema.EVENT_TYPE_STRUCTURED) {
				if utils.MatchAllOfSubString(e.Content, string(aicommon.AITaskState_Completed)) {
					gotSatisfied = true
				}
			}

			if gotResult && gotKnowledge && gotSatisfied && gotSyncKnowledge {
				break LOOP
			}
		case <-ctx.Done():
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
	if !gotSyncKnowledge {
		t.Fatal("Expected sync knowledge event")
	}
}
