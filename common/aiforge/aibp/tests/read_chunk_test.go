package tests

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func init() {
	aiforge.RegisterForgeExecutor("read-chunk-test-2", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aicommon.ConfigOption) (*aiforge.ForgeResult, error) {
		od, err := aiforge.NewForgeBlueprint(
			"read-chunk-test-2",
			aiforge.WithInitializePrompt(`分析文件中的敏感信息和外部链接是一个重要工作，你需要分析用户的文件，但是文件大小会非常大，你一次可以处理的上下文不多，所以你需要多次读取文件的内容，直到你找到你需要的信息。`),
			aiforge.WithPersistentPrompt(`在我们的敏感信息分析中，我们需要找到文件中的敏感信息和外部链接。同时为了方便进一步辐射敏感信息寻找范围，你需要额外列出访问内容中的 URL 和链接。请保证文件完成分析完成。`),
			aiforge.WithTools(yakscripttools.GetYakScriptAiTools(
				"read_file_chunk", "query_file_meta",
			)...),
		).CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		return nil, od.Run()
	})
}

//go:embed testsdata/abc.html
var testHtmlFile []byte

func TestReadChunk(t *testing.T) {
	filename := consts.TempFileFast(string(testHtmlFile))
	log.Infof("prepare file: %v", filename)
	aiforge.ExecuteForge("read-chunk-test-2", context.Background(), []*ypb.ExecParamItem{
		{Key: "query", Value: filename},
	}, aicommon.WithAgreeYOLO(), aicommon.WithDebugPrompt(), aicommon.WithAICallback(aiforge.GetOpenRouterAICallback()))
}
