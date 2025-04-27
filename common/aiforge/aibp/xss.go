package aibp

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed xss_prompts/init.txt
var xssInitPrompt string

//go:embed xss_prompts/persistent.txt
var xssPersistentPrompt string

func init() {
	aiforge.RegisterForgeExecutor("xss", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aid.Option) (*aiforge.ForgeResult, error) {
		ins, err := aiforge.NewForgeBlueprint(
			"xss",
			aiforge.WithInitializePrompt(xssInitPrompt),
			aiforge.WithPersistentPrompt(xssPersistentPrompt),
			aiforge.WithToolKeywords([]string{"fs", "http"}),
			aiforge.WithTools(yakscripttools.GetYakScriptAiTools("do_http", "grep", "read_file_chunk", "read_file_lines")...),
		).CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		if err := ins.Run(); err != nil {
			return nil, err
		}
		return nil, nil
	})
}
