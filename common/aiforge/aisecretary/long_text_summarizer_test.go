package aisecretary

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

//go:embed long_text_summarizer_data/药.txt
var medicineContent string

//go:embed long_text_summarizer_data/我的叔叔于勒.txt
var monOncleJules string

func TestLongText_summarizer(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"long-text-summarizer",
		context.Background(),
		[]*ypb.ExecParamItem{
			//{Key: "text", Value: monOncleJules},
			{Key: "file", Value: "C:\\Users\\Rookie\\home\\code\\yaklang\\common\\aiforge\\aisecretary\\long_text_summarizer_data\\我的叔叔于勒.txt"},
		},
		aid.WithDebugPrompt(true),
		aid.WithAICallback(aiforge.GetHoldAICallback()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	t.Logf("Result: %s", result.Formated)
}
