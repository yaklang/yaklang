package tests

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
)

func TestDisableForge(t *testing.T) {
	t.Run("without disable forge - should expose forge action schema", func(t *testing.T) {
		var prompt string
		callback := mockedAnswerThenFinish("Hello, world!")
		engine := newTestAIEngine(t, func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if prompt == "" && aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
				prompt = req.GetPrompt()
			}
			return callback(i, req)
		})
		defer engine.Close()

		engine.SendMsg("Hello, world!")
		engine.WaitTaskFinish()

		if !strings.Contains(prompt, `"blueprint_payload": {`) {
			t.Fatal("prompt action schema should expose blueprint_payload when forge is enabled")
		}
	})

	t.Run("with disable forge - should omit forge action schema", func(t *testing.T) {
		var prompt string
		callback := mockedAnswerThenFinish("Hello, world!")
		engine := newTestAIEngine(t, func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if prompt == "" && aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
				prompt = req.GetPrompt()
			}
			return callback(i, req)
		}, aiengine.WithDisableAIForge(true))
		defer engine.Close()

		engine.SendMsg("Hello, world!")
		engine.WaitTaskFinish()

		if strings.Contains(prompt, `"blueprint_payload": {`) {
			t.Fatal("prompt action schema should omit blueprint_payload when forge is disabled")
		}
	})
}
