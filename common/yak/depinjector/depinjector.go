package depinjector

import (
	"io"

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
		ChatToAiFunc: func(msg string) (io.Reader, error) {
			return ai.ChatStream(msg)
		},
	})
	omnisearch.RegisterSearchers(aiSearchTools)
}
