package tests

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	// forgeToolKeywords, err := searchtools.ToolKeywordSummary(
	// 	"你正在帮助用户处理一些文本任务，任务来源可能是，CTF这类黑客竞赛，一般会涉及到一些编码解码工作",
	// 	buildinaitools.GetSuggestedTools(),
	// 	20,
	// 	func(msg string) (io.Reader, error) {
	// 		reader, writer := io.Pipe()
	// 		go func() {
	// 			defer writer.Close()
	// 			opts := []aispec.AIConfigOption{
	// 				aispec.WithStreamHandler(func(c io.Reader) {
	// 					io.Copy(writer, c)
	// 				}),
	// 				aispec.WithReasonStreamHandler(func(c io.Reader) {
	// 					io.Copy(writer, c)
	// 				}),
	// 			}
	// 			_, err := ai.Chat(msg, opts...)
	// 			if err != nil {
	// 				log.Errorf("chat error: %v", err)
	// 			}
	// 		}()
	// 		return reader, nil
	// 	},
	// )
	// if err != nil {
	// 	log.Errorf("[ForgeBlueprint.KeywordPrompt] %v", err)
	// }
	// log.Infof("forgeToolKeywords: %v", forgeToolKeywords)
	aiforge.RegisterForgeExecutor("codec-test-1", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aicommon.ConfigOption) (*aiforge.ForgeResult, error) {
		od, err := aiforge.NewForgeBlueprint(
			"codec-test1",
			aiforge.WithInitializePrompt(`你正在帮助用户处理一些文本任务，任务来源可能是，CTF这类黑客竞赛，一般会涉及到一些编码解码工作，请你考虑清楚问题回答

 {{ .Forge.UserParams }}
`),
			aiforge.WithPersistentPrompt("处理任务的时候，注意深度，使用工具如果失败，无法达到你的目的，需要及时调整工具参数。"),
			// aiforge.WithTools(yakscripttools.GetYakScriptAiTools("encode", "decode")...),
			// aiforge.WithToolKeywords(forgeToolKeywords),
		).CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		return nil, od.Run()
	})
}

func TestCodecTest1(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	aiforge.ExecuteForge(
		"codec-test-1",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "query", Value: "帮我解答一个CTF题目，题目类型是加解密，附件内容是：RzRaVE1PQldHVTNFR05TREdaQ0RNTUpYR1EzREtOWlRHNUJETU1SVEdRWkRJTVpUR0kyREtSUlRHRVpUTU5LR0dNWlRHTVJWSVlaVE1NWlVHNUNBPT09PQ=="},
		},
		aicommon.WithAgreeYOLO(),
		aicommon.WithAiToolsSearchTool(),
		aicommon.WithAICallback(aiforge.GetOpenRouterAICallbackWithProxy()),
		aicommon.WithDebugPrompt(true),
	)
}
