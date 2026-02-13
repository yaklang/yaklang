package searchtools

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const SearchToolName = "tools_search"
const SearchForgeName = "aiforge_search"
const SearchCapabilitiesName = "search_capabilities"

// CreateAISearchTools creates a single-category search tool (legacy, kept for compatibility).
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

			if !utils.IsNil(searcher) {
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
			}
			var buf bytes.Buffer

			tools, err := yakit.SearchAIYakTool(consts.GetGormProfileDatabase(), query)
			if err != nil {
				return nil, utils.Errorf("search AIYakTool failed: %v", err)
			}
			for _, i := range tools {
				suffix := ""
				if i.VerboseName != "" {
					suffix = fmt.Sprintf(" (%s)", i.VerboseName)
				}
				buf.WriteString(fmt.Sprintf("- `%v`: %v%v\n", i.Name, i.Description, suffix))
			}

			results := buf.String()
			return results, nil
		}),
	)

	if err != nil {
		log.Errorf("register omni_search tool failed: %v", err)
		return nil, err
	}
	return factory.Tools(), nil
}

// SkillSearchFunc is a callback that searches skills by keyword and returns matched skill name+description pairs.
type SkillSearchFunc func(query string, limit int) ([]map[string]string, error)

// SearchCapabilitiesConfig holds configuration for the unified search_capabilities tool.
type SearchCapabilitiesConfig struct {
	ToolSearcher  AISearcher[*aitool.Tool]
	ToolsGetter   func() []*aitool.Tool
	ForgeSearcher AISearcher[*schema.AIForge]
	ForgesGetter  func() []*schema.AIForge
	SkillSearchFn SkillSearchFunc
}

// CreateSearchCapabilitiesTool creates the unified search_capabilities tool
// that searches tools, forges, AND skills simultaneously.
func CreateSearchCapabilitiesTool(cfg *SearchCapabilitiesConfig) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(
		SearchCapabilitiesName,
		aitool.WithDescription(
			"Search all available capabilities: tools, AI forges (blueprints), and skills. "+
				"Use this to discover tools not shown in the core list, find AI blueprints for complex tasks, "+
				"or locate skills for specialized knowledge. Returns categorized results.",
		),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Search keywords. Describe what you need using natural language, e.g. 'port scanning', 'vulnerability detection', 'encode base64'."),
		),
		aitool.WithStringParam("category",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("Filter search scope: 'all' (default), 'tools', 'forge', or 'skills'."),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			query := params.GetString("query")
			category := strings.ToLower(strings.TrimSpace(params.GetString("category")))
			if category == "" {
				category = "all"
			}

			var results strings.Builder
			results.WriteString(fmt.Sprintf("## Search Results for: %s\n\n", query))
			hasResults := false

			// 1. Search Tools
			if category == "all" || category == "tools" {
				toolResults := searchTools(cfg, query)
				if toolResults != "" {
					results.WriteString(toolResults)
					hasResults = true
				}
			}

			// 2. Search Forges
			if category == "all" || category == "forge" || category == "forges" {
				forgeResults := searchForges(cfg, query)
				if forgeResults != "" {
					results.WriteString(forgeResults)
					hasResults = true
				}
			}

			// 3. Search Skills
			if category == "all" || category == "skills" || category == "skill" {
				skillResults := searchSkills(cfg, query)
				if skillResults != "" {
					results.WriteString(skillResults)
					hasResults = true
				}
			}

			if !hasResults {
				results.WriteString("No matching capabilities found. Try different keywords or broaden your search.\n")
			}

			return results.String(), nil
		}),
	)
	if err != nil {
		log.Errorf("register search_capabilities tool failed: %v", err)
		return nil, err
	}
	return factory.Tools(), nil
}

func searchTools(cfg *SearchCapabilitiesConfig, query string) string {
	var buf strings.Builder

	// Try RAG searcher first
	if cfg != nil && !utils.IsNil(cfg.ToolSearcher) && cfg.ToolsGetter != nil {
		rspTools, err := cfg.ToolSearcher(query, cfg.ToolsGetter())
		if err == nil && len(rspTools) > 0 {
			buf.WriteString("### Matched Tools\n")
			for _, tool := range rspTools {
				name := tool.GetName()
				if vn := tool.GetVerboseName(); vn != "" {
					name = vn + " (" + tool.GetName() + ")"
				}
				desc := tool.GetDescription()
				if len(desc) > 200 {
					desc = desc[:200] + "..."
				}
				buf.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
			}
			buf.WriteString("\n")
			return buf.String()
		}
	}

	// Fallback to BM25 DB search
	db := consts.GetGormProfileDatabase()
	if db != nil {
		tools, err := yakit.SearchAIYakToolBM25(db, &yakit.AIYakToolFilter{Keywords: query}, 10, 0)
		if err != nil {
			log.Warnf("search_capabilities: BM25 tool search failed: %v", err)
		} else if len(tools) > 0 {
			buf.WriteString("### Matched Tools\n")
			for _, tool := range tools {
				name := tool.Name
				if tool.VerboseName != "" {
					name = tool.VerboseName + " (" + tool.Name + ")"
				}
				desc := tool.Description
				if len(desc) > 200 {
					desc = desc[:200] + "..."
				}
				buf.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
			}
			buf.WriteString("\n")
			return buf.String()
		}
	}

	return ""
}

func searchForges(cfg *SearchCapabilitiesConfig, query string) string {
	var buf strings.Builder

	// Try RAG searcher first
	if cfg != nil && !utils.IsNil(cfg.ForgeSearcher) && cfg.ForgesGetter != nil {
		rspForges, err := cfg.ForgeSearcher(query, cfg.ForgesGetter())
		if err == nil && len(rspForges) > 0 {
			buf.WriteString("### Matched AI Forges (Blueprints)\n")
			for _, forge := range rspForges {
				name := forge.GetName()
				if vn := forge.GetVerboseName(); vn != "" {
					name = vn + " (" + forge.GetName() + ")"
				}
				desc := forge.GetDescription()
				if len(desc) > 200 {
					desc = desc[:200] + "..."
				}
				buf.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
			}
			buf.WriteString("\n")
			return buf.String()
		}
	}

	// Fallback to BM25 DB search
	db := consts.GetGormProfileDatabase()
	if db != nil {
		forges, err := yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{Keywords: query}, 10, 0)
		if err != nil {
			log.Warnf("search_capabilities: BM25 forge search failed: %v", err)
		} else if len(forges) > 0 {
			buf.WriteString("### Matched AI Forges (Blueprints)\n")
			for _, forge := range forges {
				name := forge.ForgeName
				if forge.ForgeVerboseName != "" {
					name = forge.ForgeVerboseName + " (" + forge.ForgeName + ")"
				}
				desc := forge.Description
				if len(desc) > 200 {
					desc = desc[:200] + "..."
				}
				buf.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
			}
			buf.WriteString("\n")
			return buf.String()
		}
	}

	return ""
}

func searchSkills(cfg *SearchCapabilitiesConfig, query string) string {
	if cfg == nil || cfg.SkillSearchFn == nil {
		return ""
	}

	skills, err := cfg.SkillSearchFn(query, 5)
	if err != nil {
		log.Warnf("search_capabilities: skill search failed: %v", err)
		return ""
	}
	if len(skills) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("### Matched Skills\n")
	for _, skill := range skills {
		name := skill["Name"]
		desc := skill["Description"]
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		buf.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
	}
	buf.WriteString("\n")
	return buf.String()
}
