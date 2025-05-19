package aibp

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
)

func TestNetScan(t *testing.T) {
	result, err := ExecuteForge(
		"netscan",
		"www.example.com",
		aid.WithAgreeYOLO(),
		aid.WithDebugPrompt(true),
		aid.WithAiToolsSearchTool(),
	)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}
