package loop_coordinator

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func (s *Session) configureToolSearch() error {
	manager := s.AiToolManager.ForkForSession()
	if err := manager.EnableAIToolSearch(func(query string, tools []*aitool.Tool) ([]*aitool.Tool, error) {
		catalog := omap.NewOrderedMap[string, []string](nil)
		byName := map[string]*aitool.Tool{}
		for _, tool := range tools {
			catalog.Set(tool.Name, tool.GetKeywords())
			byName[tool.Name] = tool
		}
		matches, err := s.HandleSearch(query, catalog)
		if err != nil {
			return nil, err
		}
		result := make([]*aitool.Tool, 0, len(matches))
		for _, match := range matches {
			result = append(result, byName[match.Key])
		}
		return result, nil
	}); err != nil {
		return err
	}
	return aicommon.WithAiToolManager(manager)(s.Config)
}

func (s *Session) HandleSearch(query string, items *omap.OrderedMap[string, []string]) ([]*searchtools.KeywordSearchResult, error) {
	catalog := map[string][]string{}
	items.ForEach(func(name string, keywords []string) bool { catalog[name] = keywords; return true })
	data, _ := json.Marshal(map[string]any{"query": query, "catalog": catalog})
	result, err := s.InvokeLiteForge("Match the query against the catalog. Return only listed tool names. An empty matches array is valid.\n"+string(data), &aicommon.LiteForgeInvokeRequest{Context: s.GetContext(), ActionName: "keyword_search", Outputs: []aitool.ToolOption{aitool.WithStructArrayParam("matches", []aitool.PropertyOption{aitool.WithParam_Required()}, nil, aitool.WithStringParam("tool", aitool.WithParam_Required()), aitool.WithStringArrayParam("matched_keywords"))}})
	if err != nil {
		return nil, err
	}
	matches := []*searchtools.KeywordSearchResult{}
	seen := map[string]bool{}
	for _, match := range result.GetInvokeParamsArray("matches") {
		name := match.GetString("tool")
		if !items.Have(name) {
			return nil, fmt.Errorf("unknown tool %q", name)
		}
		if !seen[name] {
			matches = append(matches, &searchtools.KeywordSearchResult{Key: name, MatchedKeywords: match.GetStringSlice("matched_keywords")})
			seen[name] = true
		}
	}
	return matches, nil
}
