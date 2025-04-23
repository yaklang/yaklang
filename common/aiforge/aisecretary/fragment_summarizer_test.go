package aisecretary

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestFragment_summarizer(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"fragment-summarizer",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "textSnippet", Value: `近年来，人工智能（AI）在医疗领域的应用迅速扩展。深度学习算法已能通过医学影像（如X光、MRI）辅助诊断疾病，准确率接近专业医生。例如，Google Health 开发的AI系统在乳腺癌筛查中的准确率达到94%，高于人类放射科医师的平均水平（89%）。然而，这些系统依赖大量标注数据训练，而医疗数据的隐私性和稀缺性限制了模型的泛化能力。`},
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
