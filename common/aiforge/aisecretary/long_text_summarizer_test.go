package aisecretary

import (
	"context"
	_ "embed"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed long_text_summarizer_data/药.txt
var medicineContent string

//go:embed long_text_summarizer_data/我的叔叔于勒.txt
var monOncleJules string

func TestLongText_summarizer(t *testing.T) {
	tempDir := t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer tempFile.Close()

	if _, err := tempFile.WriteString(monOncleJules); err != nil {
		t.Fatal(err)
	}

	if err := tempFile.Sync(); err != nil {
		t.Fatal(err)
	}

	if _, err := tempFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	result, err := aiforge.ExecuteForge(
		"long-text-summarizer",
		context.Background(),
		[]*ypb.ExecParamItem{
			//{Key: "text", Value: monOncleJules},
			{Key: "file", Value: tempFile.Name()},
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
