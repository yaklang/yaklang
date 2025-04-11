package depinjector

import (
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/omnisearch"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
)

func DependencyInject() {
	injectAiTools()
}

var _ ostype.SearchClient = &searchtools.AiToolsSearchClient{}

func injectAiTools() {
	aiSearchTools := searchtools.NewAiToolsSearchClient(buildinaitools.GetAllTools, &searchtools.AiToolsSearchClientConfig{
		SearchType: "ai",
		CallAiFunc: func(msg string) (string, error) {
			rsp, err := ai.Chat(msg)
			if err != nil {
				return "", err
			}
			return rsp, nil
		},
	})
	omnisearch.RegisterSearchers(aiSearchTools)
}
