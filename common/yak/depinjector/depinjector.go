package depinjector

import (
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

func DependencyInject() {
	mcp.RegisterNewLocalClient(func(locals ...bool) (mcp.YakClientInterface, error) {
		client, err := yakgrpc.NewLocalClient(locals...)
		if err != nil {
			return nil, err
		}
		v, ok := client.(mcp.YakClientInterface)
		if !ok {
			return nil, utils.Error("failed to cast client to yakgrpc.Client")
		}
		return v, nil
	})
	yak.AIEngineExports = aiengine.Exports
}
