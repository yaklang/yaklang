package aibp

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed netscan/init.txt
var netscanInitPrompt string

//go:embed netscan/persistent.txt
var netscanPersistentPrompt string

func init() {
	aiforge.RegisterForgeExecutor(`netscan`, func(ctx context.Context, items []*ypb.ExecParamItem, option ...aid.Option) (*aiforge.ForgeResult, error) {
		ins, err := aiforge.NewForgeBlueprint(
			"netscan",
			aiforge.WithInitializePrompt(netscanInitPrompt),
			aiforge.WithPersistentPrompt(netscanPersistentPrompt),
			aiforge.WithTools(yakscripttools.GetYakScriptAiTools("http", "pentest", "net", "fs", "dns", "codec", "risk", "tls")...),
		).CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		return nil, ins.Run()
	})
}
