package tests

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
	aidmock "github.com/yaklang/yaklang/common/aiengine/tests/aid_mock"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDisableForge(t *testing.T) {
	// 查找 forge 列表
	db := consts.GetGormProfileDatabase()
	_, forges, err := yakit.QueryAIForge(db, &ypb.AIForgeFilter{}, &ypb.Paging{})
	if err != nil {
		t.Fatalf("failed to query ai forge: %v", err)
	}

	t.Run("without disable forge - should contain forge info", func(t *testing.T) {
		// 验证提示词中包含至少一个 forge 信息
		var prompt string
		engine := newTestAIEngine(t, func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt = req.GetPrompt()
			return aidmock.HelloWorldScenario.GetAICallbackType()(i, req)
		})
		defer engine.Close()

		engine.SendMsg("Hello, world!")
		engine.WaitTaskFinish()

		println("Prompt without disable forge:", prompt)
		// 提示词中应该包含至少一个 forge 信息
		var hasAnyForgeInfo bool
		for _, forge := range forges {
			hasName := strings.Contains(prompt, forge.ForgeName)
			hasContent := strings.Contains(prompt, forge.ForgeContent)
			hasDescription := strings.Contains(prompt, forge.Description)
			if hasName && hasContent && hasDescription {
				hasAnyForgeInfo = true
				break
			}
		}
		if !hasAnyForgeInfo {
			t.Fatalf("prompt should contain at least one forge info, but got none")
		}
	})

	t.Run("with disable forge - should not contain any forge info", func(t *testing.T) {
		// 验证提示词中不包含 forge 列表
		var prompt string
		engine := newTestAIEngine(t, func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt = req.GetPrompt()
			return aidmock.HelloWorldScenario.GetAICallbackType()(i, req)
		}, aiengine.WithDisableAIForge(true))
		defer engine.Close()

		engine.SendMsg("Hello, world!")
		engine.WaitTaskFinish()

		println("Prompt with disable forge:", prompt)
		// 提示词中不包含任何 forge 信息
		for _, forge := range forges {
			var hasThisForgeInfo bool

			hasName := strings.Contains(prompt, forge.ForgeName)
			hasContent := strings.Contains(prompt, forge.ForgeContent)
			hasDescription := strings.Contains(prompt, forge.Description)
			hasThisForgeInfo = hasName && hasContent && hasDescription
			if hasThisForgeInfo {
				t.Fatalf("prompt should not contain forge info, but contains: %s", forge.ForgeName)
			}
		}
	})
}
