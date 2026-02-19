package loop_intent

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// searchCapabilitiesAction creates the query_capabilities action
// that searches for tools, forges, and focus modes matching the user's intent.
// Renamed from "search_capabilities" to "query_capabilities" to avoid conflict
// with the global built-in @action "search_capabilities".
var searchCapabilitiesAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeSearchCapabilitiesAction(r)
}

func makeSearchCapabilitiesAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "搜索与用户意图匹配的可用能力（工具、AI 蓝图、专注模式）。" +
		"使用 BM25 Trigram 搜索工具和蓝图，关键词匹配专注模式。/ " +
		"Search for available capabilities (tools, AI forges, focus modes) matching the user's intent."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("search_query",
			aitool.WithParam_Description("搜索关键词，从用户输入中提取核心动作词和领域术语。/ Keywords to search for relevant capabilities."),
			aitool.WithParam_Required(true),
		),
	}

	return reactloops.WithRegisterLoopActionWithStreamField(
		"query_capabilities",
		desc,
		toolOpts,
		[]*reactloops.LoopStreamField{
			{FieldName: "search_query", AINodeId: "intent"},
		},
		// Verifier
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			query := strings.TrimSpace(action.GetString("search_query"))
			if query == "" {
				return utils.Error("search_query is required for capability search")
			}
			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			query := strings.TrimSpace(action.GetString("search_query"))
			log.Infof("intent loop: searching capabilities with query: %s", query)

			var results strings.Builder
			results.WriteString(fmt.Sprintf("## Capability Search Results for: %s\n\n", query))

			// 1. Search AIYakTool via BM25 trigram
			db := consts.GetGormProfileDatabase()
			if db != nil {
				loop.LoadingStatus("Start to load bm25+keyword search results for tools and AI forges... / 开始加载工具和AI蓝图的BM25+关键词搜索结果...")
				tools, err := yakit.SearchAIYakToolBM25(db, &yakit.AIYakToolFilter{
					Keywords: query,
				}, 10, 0)
				if err != nil {
					log.Warnf("intent loop: BM25 tool search failed: %v", err)
				} else if len(tools) > 0 {
					results.WriteString("### Matched Tools\n")
					for _, tool := range tools {
						name := tool.Name
						if tool.VerboseName != "" {
							name = tool.VerboseName + " (" + tool.Name + ")"
						}
						desc := utils.ShrinkString(tool.Description, 200)
						results.WriteString(fmt.Sprintf("- **%s**: %s", name, desc))
						if tool.Keywords != "" {
							results.WriteString(fmt.Sprintf(" [keywords: %s]", tool.Keywords))
						}
						results.WriteString("\n")
					}
					results.WriteString("\n")
					log.Infof("intent loop: found %d tools via BM25", len(tools))

					// Store matched tool names for later reference
					var toolNames []string
					for _, t := range tools {
						toolNames = append(toolNames, t.Name)
					}
					loop.Set("matched_tool_names", strings.Join(toolNames, ","))
				} else {
					results.WriteString("### Tools\nNo matching tools found.\n\n")
				}

				// 2. Search AI Forges via BM25 trigram (with LIKE fallback for short queries)
				forges, err := yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{
					Keywords: query,
				}, 10, 0)
				if err != nil {
					log.Warnf("intent loop: BM25 forge search failed: %v", err)
				}
				if len(forges) > 0 {
					results.WriteString("### Matched AI Forges (Blueprints)\n")
					for _, forge := range forges {
						name := forge.ForgeName
						if forge.ForgeVerboseName != "" {
							name = forge.ForgeVerboseName + " (" + forge.ForgeName + ")"
						}
						desc := utils.ShrinkString(forge.Description, 200)
						results.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
					}
					results.WriteString("\n")
					log.Infof("intent loop: found %d forges", len(forges))

					var forgeNames []string
					for _, f := range forges {
						forgeNames = append(forgeNames, f.ForgeName)
					}
					loop.Set("matched_forge_names", strings.Join(forgeNames, ","))
				} else {
					results.WriteString("### AI Forges\nNo matching forges found.\n\n")
				}
			} else {
				results.WriteString("### Tools & Forges\nDatabase not available.\n\n")
			}

			// 3. Search Skills via SkillLoader (if available)
			searchSkillsFromLoader(r, query, &results, loop)

			// 4. Search registered loop metadata
			matchedLoops := searchLoopMetadata(query)
			if len(matchedLoops) > 0 {
				results.WriteString("### Matched Focus Modes\n")
				for _, meta := range matchedLoops {
					results.WriteString(fmt.Sprintf("- **%s**: %s\n", meta.Name, meta.Description))
				}
				results.WriteString("\n")
				log.Infof("intent loop: found %d matching focus modes", len(matchedLoops))

				var loopNames []string
				for _, l := range matchedLoops {
					loopNames = append(loopNames, l.Name)
				}
				loop.Set("matched_loop_names", strings.Join(loopNames, ","))
			} else {
				// Also include available focus modes for reference
				availableModes := loop.Get("available_focus_modes")
				if availableModes != "" {
					results.WriteString("### Available Focus Modes (no direct match)\n")
					results.WriteString(availableModes)
					results.WriteString("\n")
				}
			}

			// Store search results
			existingResults := loop.Get("search_results")
			if existingResults != "" {
				existingResults += "\n---\n\n"
			}
			loop.Set("search_results", existingResults+results.String())

			op.Feedback(results.String())
			op.Continue()
		},
	)
}

// searchSkillsFromLoader searches skills via the SkillLoader if available in the runtime config.
// Uses type assertion to access GetSkillLoader() from the concrete Config type.
func searchSkillsFromLoader(r aicommon.AIInvokeRuntime, query string, results *strings.Builder, loop *reactloops.ReActLoop) {
	// Try to get skill loader via type assertion on the config
	type skillLoaderProvider interface {
		GetSkillLoader() aiskillloader.SkillLoader
	}
	cfg := r.GetConfig()
	provider, ok := cfg.(skillLoaderProvider)
	if !ok {
		return
	}
	skillLoader := provider.GetSkillLoader()
	if skillLoader == nil || !skillLoader.HasSkills() {
		return
	}

	// Use AllSkillMetas and do manual substring matching (same as AutoSkillLoader.SearchSkills)
	allMetas := skillLoader.AllSkillMetas()
	queryLower := strings.ToLower(query)
	queryTokens := strings.Fields(queryLower)

	var matchedSkills []*aiskillloader.SkillMeta
	for _, meta := range allMetas {
		searchText := strings.ToLower(meta.Name + " " + meta.Description)

		// Full query match
		if strings.Contains(searchText, queryLower) {
			matchedSkills = append(matchedSkills, meta)
			continue
		}

		// Token-level match
		if len(queryTokens) > 1 {
			meaningfulTokens := 0
			matchCount := 0
			for _, token := range queryTokens {
				if len(token) < 2 {
					continue
				}
				meaningfulTokens++
				if strings.Contains(searchText, token) {
					matchCount++
				}
			}
			if meaningfulTokens > 0 && matchCount > 0 && matchCount >= (meaningfulTokens+1)/2 {
				matchedSkills = append(matchedSkills, meta)
			}
		}
	}

	if len(matchedSkills) > 0 {
		results.WriteString("### Matched Skills\n")
		limit := 5
		for i, skill := range matchedSkills {
			if i >= limit {
				results.WriteString(fmt.Sprintf("... and %d more skills\n", len(matchedSkills)-limit))
				break
			}
			desc := skill.Description
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			results.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name, desc))
		}
		results.WriteString("\n")
		log.Infof("intent loop: found %d matching skills", len(matchedSkills))

		var skillNames []string
		for _, s := range matchedSkills {
			skillNames = append(skillNames, s.Name)
		}
		loop.Set("matched_skill_names", strings.Join(skillNames, ","))
	}
}

// searchLoopMetadata searches registered loop metadata for keyword matches.
// LoopMetadata is in-memory (not DB-backed), so this uses token-level matching.
func searchLoopMetadata(query string) []*reactloops.LoopMetadata {
	allMeta := reactloops.GetAllLoopMetadata()
	queryLower := strings.ToLower(query)
	queryTokens := strings.Fields(queryLower)
	var matched []*reactloops.LoopMetadata

	for _, meta := range allMeta {
		if meta.IsHidden {
			continue
		}
		searchText := strings.ToLower(meta.Name + " " + meta.Description + " " + meta.UsagePrompt)

		// Full query match
		if strings.Contains(searchText, queryLower) {
			matched = append(matched, meta)
			continue
		}

		// Token-level match: require at least half of meaningful tokens
		if len(queryTokens) > 1 {
			meaningfulTokens := 0
			matchCount := 0
			for _, token := range queryTokens {
				if len(token) < 2 {
					continue
				}
				meaningfulTokens++
				if strings.Contains(searchText, token) {
					matchCount++
				}
			}
			if meaningfulTokens > 0 && matchCount > 0 && matchCount >= (meaningfulTokens+1)/2 {
				matched = append(matched, meta)
			}
		}
	}
	return matched
}
