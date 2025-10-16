package searchtools

import (
	"io"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const SearchToolName = "tools_search"
const SearchForgeName = "aiforge_search"

func CreateAISearchTools[T AISearchable](searcher AISearcher[T], searchListGetter func() []T, toolName string) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(
		toolName,
		aitool.WithDescription("Search resources or tools that can search the names of all currently supported things"),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("The name of the tool to query, can describe requirements using natural language."),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			query := params.GetString("query")
			rspTools, err := searcher(query, searchListGetter())
			if err != nil {
				return nil, utils.Errorf("search failed: %v", err)
			}
			result := []any{}
			for _, tool := range rspTools {
				result = append(result, map[string]string{
					"Name":        tool.GetName(),
					"Description": tool.GetDescription(),
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
