package aid

import (
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/rag/rag_search_tool"
	"github.com/yaklang/yaklang/common/schema"
)

// WithAiToolsSearchTool enables shared AI-assisted tool discovery.
func WithAiToolsSearchTool() aicommon.ConfigOption {
	return func(c *aicommon.Config) error {
		aiChatFunc := func(prompt string) (io.Reader, error) {
			response, err := ai.Chat(prompt)
			if err != nil {
				return nil, err
			}
			return strings.NewReader(response), nil
		}

		aiToolSearcher := rag_search_tool.NewComprehensiveSearcher[*aitool.Tool](rag_search_tool.AIToolVectorIndexName, aiChatFunc)
		return aicommon.WithAiToolManagerOptions(buildinaitools.WithSearchToolEnabled(true),
			buildinaitools.WithAIToolsSearcher(aiToolSearcher))(c)
	}
}

// WithAiForgeSearchTool enables shared AI-assisted Forge discovery.
func WithAiForgeSearchTool() aicommon.ConfigOption {
	return func(c *aicommon.Config) error {
		aiChatFunc := func(prompt string) (io.Reader, error) {
			response, err := ai.Chat(prompt)
			if err != nil {
				return nil, err
			}
			return strings.NewReader(response), nil
		}

		forgeSearcher := rag_search_tool.NewComprehensiveSearcher[*schema.AIForge](rag_search_tool.ForgeVectorIndexName, aiChatFunc)
		return aicommon.WithAiToolManagerOptions(
			buildinaitools.WithForgeSearchToolEnabled(true),
			buildinaitools.WithAiForgeSearcher(forgeSearcher))(c)
	}
}
