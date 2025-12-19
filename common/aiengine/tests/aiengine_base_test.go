package tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/aiengine"
	aidmock "github.com/yaklang/yaklang/common/aiengine/tests/aid_mock"
	"github.com/yaklang/yaklang/common/schema"
	"gotest.tools/v3/assert"

	// import aireact to register NewReAct factory
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
)

// hello world test
func TestHelloWorld(t *testing.T) {
	aiCallBack := aidmock.HelloWorldScenario.GetAICallbackType()

	aiRsp := ""
	engine := newTestAIEngine(t, aiCallBack, aiengine.WithOnStream(func(react aicommon.ReActIF, event *schema.AiOutputEvent, NodeId string, data []byte) {
		if NodeId == "re-act-loop-answer-payload" {
			aiRsp += string(data)
		}
	}))
	defer engine.Close()

	engine.SendMsg("Hello, world!")
	assert.Equal(t, aiRsp, "Hello, world!")
}

func newTestAIEngine(t *testing.T, mockCallback func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error), options ...aiengine.AIEngineConfigOption) *aiengine.AIEngine {
	// 添加 mock AI 回调
	allOptions := append([]aiengine.AIEngineConfigOption{
		aiengine.WithAICallback(mockCallback),
		aiengine.WithDisableMCPServers(true),
		aiengine.WithExtOptions(
			aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
			aicommon.WithEnableSelfReflection(false),
		),
		aiengine.WithSessionID(uuid.New().String()),
	}, options...)

	engine, err := aiengine.NewAIEngine(allOptions...)
	if err != nil {
		t.Fatalf("failed to create test AI engine: %v", err)
	}
	return engine
}
