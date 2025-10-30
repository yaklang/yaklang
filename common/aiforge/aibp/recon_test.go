package aibp

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/aiforge"
)

func TestRecon(t *testing.T) {
	result, err := ExecuteForge(
		"recon",
		"www.example.com",
		aicommon.WithAgreeYOLO(),
		aicommon.WithDebugPrompt(true),
		aicommon.WithAICallback(aiforge.GetOpenRouterAICallbackWithProxy()),
	)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}
