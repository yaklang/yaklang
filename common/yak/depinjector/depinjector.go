package depinjector

import (
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/depinjector/aiforge"

	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
	yakit.SearchPluginIdsFunc = plugins_rag.SearchPluginIds
	enhancesearch.Simpleliteforge = aiforge.SimpleAiForgeIns
	knowledgebase.Simpleliteforge = aiforge.SimpleAiForgeIns
}
