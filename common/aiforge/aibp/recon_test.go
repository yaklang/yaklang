package aibp

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
)

func TestRecon(t *testing.T) {
	result, err := ExecuteForge(
		"recon",
		"www.example.com",
		aid.WithAgreeYOLO(),
		aid.WithDebugPrompt(true),
		aid.WithAICallback(aiforge.GetOpenRouterAICallbackWithProxy()),
	)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}
