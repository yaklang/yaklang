package searchtools

import (
	"io"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const SeatchToolName = "tools_search"

func CreateAiToolsSearchTools(toolsGetter func() []*aitool.Tool, searcher AiToolSearcher) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(
		SeatchToolName,
		aitool.WithDescription("Search tool that can search the names of all currently supported tools."),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("The name of the tool to query, can describe tool requirements using natural language."),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			query := params.GetString("query")

			rspTools, err := searcher(&ToolSearchRequest{
				Query: query,
			})
			if err != nil {
				return nil, utils.Errorf("search failed: %v", err)
			}

			result := []any{}
			for _, tool := range rspTools {
				result = append(result, map[string]string{
					"Name":        tool.Name,
					"Description": tool.Description,
				})
			}
			return result, nil
		}),
	)

	if err != nil {
		log.Errorf("register omni_search tool failed: %v", err)
		return nil, err
	}
	return factory.Tools(), nil
}
