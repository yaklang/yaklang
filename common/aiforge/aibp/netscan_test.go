package aibp

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
)

func TestNetScan(t *testing.T) {
	result, err := ExecuteForge(
		"netscan",
		"www.example.com",
		aicommon.WithAgreeYOLO(),
		aicommon.WithDebugPrompt(true),
		aid.WithAiToolsSearchTool(),
	)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}
