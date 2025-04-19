package yaklangmaster

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed yaklang_writer_prompts/init.txt
var writerInitPrompt string

//go:embed yaklang_writer_prompts/persistent.txt
var writerPersistentPrompt string

func init() {
	aiforge.RegisterForgeExecutor("yaklang-writer", func(
		ctx context.Context,
		items []*ypb.ExecParamItem,
		option ...aid.Option) (*aiforge.ForgeResult, error) {
		bp := aiforge.NewForgeBlueprint(
			"yaklang-writer",
			aiforge.WithInitializePrompt(writerInitPrompt),
			aiforge.WithPersistentPrompt(writerPersistentPrompt),
		)
		ord, err := bp.CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		err = ord.Run()
		if err != nil {
			return nil, err
		}
		return nil, nil
	})
}
