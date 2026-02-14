package searchtools

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"

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

	// OnSearchCompleted is called after each successful search.
	// It can be used to add timeline entries (e.g., via AddToTimeline) to inform the AI
	// that search_capabilities has already been called and should not be called again.
	// Parameters: query string, resultSummary string
	OnSearchCompleted func(query string, resultSummary string)
}

// CreateSearchCapabilitiesTool creates the unified search_capabilities tool
// that searches tools, forges, AND skills simultaneously.
// It tracks previous searches and prevents infinite loops by refusing duplicate queries
// and notifying the AI via OnSearchCompleted callback (used for AddToTimeline).
func CreateSearchCapabilitiesTool(cfg *SearchCapabilitiesConfig) ([]*aitool.Tool, error) {
	// Track searched queries to prevent repeated calls forming a loop
	var searchedQueries sync.Map

	factory := aitool.NewFactory()
	err := factory.RegisterTool(
		SearchCapabilitiesName,
		aitool.WithDescription(
			"Search all available capabilities: tools, AI forges (blueprints), and skills. "+
				"Use this to discover tools not shown in the core list, find AI blueprints for complex tasks, "+
				"or locate skills for specialized knowledge. Returns categorized results. "+
				"IMPORTANT: Each query should only be searched ONCE. Do NOT call this tool repeatedly with the same or similar query.",
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

			// Check if this query (or similar) has been searched before
			normalizedQuery := strings.ToLower(strings.TrimSpace(query))
			if prevResult, loaded := searchedQueries.Load(normalizedQuery); loaded {
				// Already searched this query - refuse and return previous summary
				refusalMsg := fmt.Sprintf(
					"## search_capabilities: DUPLICATE QUERY REJECTED\n\n"+
						"The query %q has already been searched. Results were provided previously.\n"+
						"**DO NOT call search_capabilities again.** Use the previously returned results to proceed with your task.\n\n"+
						"Previous result summary:\n%s", query, prevResult.(string))
				log.Infof("search_capabilities: rejected duplicate query %q", query)
				return refusalMsg, nil
			}

			var results strings.Builder
			results.WriteString(fmt.Sprintf("## Search Results for: %s\n\n", query))
			hasResults := false

			// Collect matched names for timeline summary
			var matchedToolNames []string
			var matchedForgeNames []string
			var matchedSkillNames []string

			// 1. Search Tools
			if category == "all" || category == "tools" {
				toolResults := searchTools(cfg, query)
				if toolResults != "" {
					results.WriteString(toolResults)
					hasResults = true
					matchedToolNames = extractMatchedNames(toolResults)
				}
			}

			// 2. Search Forges
			if category == "all" || category == "forge" || category == "forges" {
				forgeResults := searchForges(cfg, query)
				if forgeResults != "" {
					results.WriteString(forgeResults)
					hasResults = true
					matchedForgeNames = extractMatchedNames(forgeResults)
				}
			}

			// 3. Search Skills
			if category == "all" || category == "skills" || category == "skill" {
				skillResults := searchSkills(cfg, query)
				if skillResults != "" {
					results.WriteString(skillResults)
					hasResults = true
					matchedSkillNames = extractMatchedNames(skillResults)
				}
			}

			if !hasResults {
				results.WriteString("No matching capabilities found. Try different keywords or broaden your search.\n")
			}

			// Append anti-loop directive to the result
			results.WriteString("\n---\n")
			results.WriteString("**IMPORTANT**: search_capabilities has completed for this query. " +
				"Do NOT call search_capabilities again with the same or similar query. " +
				"Use the results above to proceed with your actual task.\n")

			resultStr := results.String()

			// Store the query and a brief summary to detect duplicates
			searchedQueries.Store(normalizedQuery, utils.ShrinkString(resultStr, 500))

			// Notify via callback (used for AddToTimeline in plan mode)
			// Pass structured matched names so the timeline entry can contain concrete results
			if cfg != nil && cfg.OnSearchCompleted != nil {
				cfg.OnSearchCompleted(query, buildTimelineSummary(query, matchedToolNames, matchedForgeNames, matchedSkillNames))
			}

			return resultStr, nil
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

// extractMatchedNames parses "- **name**: description" lines from search result sections
// and returns a list of matched names for timeline summary.
func extractMatchedNames(sectionResult string) []string {
	var names []string
	for _, line := range strings.Split(sectionResult, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- **") {
			continue
		}
		// Extract name from "- **name**: description"
		start := strings.Index(line, "**")
		if start < 0 {
			continue
		}
		end := strings.Index(line[start+2:], "**")
		if end < 0 {
			continue
		}
		name := line[start+2 : start+2+end]
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// buildTimelineSummary constructs a structured timeline message that includes
// the actual search results, so the AI has concrete context and won't re-search.
func buildTimelineSummary(query string, toolNames, forgeNames, skillNames []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("search_capabilities has been called for query: %q\n", query))

	hasAny := false

	if len(toolNames) > 0 {
		hasAny = true
		sb.WriteString(fmt.Sprintf("  Matched Tools: %s\n", strings.Join(toolNames, ", ")))
	}
	if len(forgeNames) > 0 {
		hasAny = true
		sb.WriteString(fmt.Sprintf("  Matched AI Forges: %s\n", strings.Join(forgeNames, ", ")))
	}
	if len(skillNames) > 0 {
		hasAny = true
		sb.WriteString(fmt.Sprintf("  Matched Skills: %s\n", strings.Join(skillNames, ", ")))
	}

	if !hasAny {
		sb.WriteString("  No matching capabilities were found for this query.\n")
	}

	sb.WriteString("  The search is complete. Do NOT call search_capabilities again for this task.\n")
	sb.WriteString("  Proceed with the matched capabilities above to accomplish your task goals.")

	return sb.String()
}
