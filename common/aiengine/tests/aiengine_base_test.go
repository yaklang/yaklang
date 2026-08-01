package tests

import (
	"bytes"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/schema"
	"gotest.tools/v3/assert"

	// import aireact to register NewReAct factory
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
)

// hello world test
func TestHelloWorld(t *testing.T) {
	aiCallBack := mockedAnswerThenFinish("Hello, world!")

	aiRsp := ""
	engine := newTestAIEngine(t, aiCallBack, aiengine.WithOnStream(func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string, data []byte) {
		if NodeId == "re-act-loop-answer-payload" && aiRsp == "" {
			aiRsp += string(data)
		}
	}))
	defer engine.Close()

	assert.NilError(t, engine.SendMsg("Hello, world!"))
	engine.WaitTaskFinish()
	assert.Equal(t, aiRsp, "Hello, world!")
}

func mockedAnswerThenFinish(answer string) aicommon.AICallbackType {
	var mu sync.Mutex
	answered := false
	return func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		if !aicommon.IsPrimaryDecisionPrompt(prompt) && aicommon.IsDirectAnswerPrompt(prompt) {
			return mockedDirectPayload(i, answer)
		}

		mu.Lock()
		shouldAnswer := !answered
		answered = true
		mu.Unlock()

		rsp := i.NewAIResponse()
		if shouldAnswer {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":` +
				strconv.Quote(answer) + `},"human_readable_thought":"provide the requested answer"}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"finish"},"human_readable_thought":"finish after the answer was delivered"}`))
		}
		rsp.Close()
		return rsp, nil
	}
}

func mockedDirectPayload(i aicommon.AICallerConfigIf, answer string) (*aicommon.AIResponse, error) {
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"directly_answer","answer_payload":` + strconv.Quote(answer) + `}`))
	rsp.Close()
	return rsp, nil
}

func newTestAIEngine(t *testing.T, mockCallback func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error), options ...aiengine.AIEngineConfigOption) *aiengine.AIEngine {
	// 添加 mock AI 回调
	allOptions := append([]aiengine.AIEngineConfigOption{
		aiengine.WithAICallback(mockCallback),
		aiengine.WithDisableMCPServers(true),
		aiengine.WithExtOptions(
			aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
			aicommon.WithDisableIntentRecognition(true),
			aicommon.WithDisableSessionTitleGeneration(true),
		),
		aiengine.WithSessionID(uuid.New().String()),
	}, options...)

	engine, err := aiengine.NewAIEngine(allOptions...)
	if err != nil {
		t.Fatalf("failed to create test AI engine: %v", err)
	}
	return engine
}
