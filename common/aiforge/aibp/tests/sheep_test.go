package tests

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	aiforge.RegisterForgeExecutor("sheep-test-1", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aicommon.ConfigOption) (*aiforge.ForgeResult, error) {
		forge := aiforge.NewForgeBlueprint(
			"sheep-test-1",
			aiforge.WithInitializePrompt("你需要在一个大片的文本中找到 blacksheep 这个词，并且说出他的大概位置 {{ .Forge.UserParams }} "),
			aiforge.WithTools(yakscripttools.GetYakScriptAiTools("grep")...),
		)
		cd, err := forge.CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		return nil, cd.Run()
	})
	aiforge.RegisterForgeExecutor("sheep-test-2", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aicommon.ConfigOption) (*aiforge.ForgeResult, error) {
		forge := aiforge.NewForgeBlueprint(
			"sheep-test-1",
			aiforge.WithInitializePrompt("你需要在一个大片的文本中找到 blacksheep 这个词，并且说出他的大概位置 {{ .Forge.UserParams }} 并且给出 blacksheep 附近的字节，展示一些上下文"),
			aiforge.WithTools(
				yakscripttools.GetYakScriptAiTools(
					"grep",
					"read_file_chunk",
				)...),
		)
		cd, err := forge.CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		return nil, cd.Run()
	})
}

func TestSheepTest(t *testing.T) {
	tempFile, err := consts.TempFile("sheep-test-1-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	tempFile.WriteString(strings.Repeat("red-sheep is impossible to find", 1000))
	tempFile.WriteString("blacksheep is a good sheep\n")
	tempFile.WriteString(strings.Repeat("whitesheep is a not good sheep", 1000))
	tempFile.Close()

	aiforge.ExecuteForge(
		"sheep-test-1",
		context.Background(),
		[]*ypb.ExecParamItem{
			{
				Key:   "query",
				Value: tempFile.Name(),
			},
		},
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return aiforge.GetOpenRouterAICallback()(config, req)
		}),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDebugPrompt(),
	)
}

func TestSheepTest2(t *testing.T) {
	tempFile, err := consts.TempFile("sheep-test-2-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	tempFile.WriteString(strings.Repeat("red-sheep is impossible to find", 1000))
	tempFile.WriteString("blacksheep is a good sheep\n")
	tempFile.WriteString(strings.Repeat("whitesheep is a not good sheep", 1000))
	tempFile.Close()

	aiforge.ExecuteForge(
		"sheep-test-2",
		context.Background(),
		[]*ypb.ExecParamItem{
			{
				Key:   "query",
				Value: tempFile.Name(),
			},
		},
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return aiforge.GetOpenRouterAICallback()(config, req)
		}),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDebugPrompt(),
	)
}
