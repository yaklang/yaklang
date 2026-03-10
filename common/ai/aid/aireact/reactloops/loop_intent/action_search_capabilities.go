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
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// appendCapDetail is a helper to collect structured capability details during search.
func appendCapDetail(details *[]capabilityDetail, name, capType, desc string) {
	*details = append(*details, capabilityDetail{
		CapabilityName: name,
		CapabilityType: capType,
		Description:    desc,
	})
}

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
			aitool.WithParam_Description("搜索关键词，从用户输入中提取核心动作词和领域术语 ,空格分割。/ Keywords to search for relevant capabilities, split by space."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("intent_summary",
			aitool.WithParam_Description("可选的简短意图标签。如果已经完成意图概括，可直接输出。/ Optional concise intent summary if already known."),
		),
		aitool.WithStringArrayParamEx("tags",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("可选的检索标签列表。如果已能提炼出领域/动作/能力标签，可直接输出。/ Optional retrieval tags if already known."),
			},
		),
		aitool.WithStringArrayParamEx("questions",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("可选的检索问题列表。如果已能提炼出关键问题表达，可直接输出。/ Optional retrieval questions if already known."),
			},
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
			// if the intent_summary is provided by the intent analysis step, store it for context , can reduce the step of intent analysis in finalize_enrichment
			if summary := action.GetString("intent_summary"); summary != "" {
				loop.Set("intent_summary", reactloops.CompactIntentSummary(summary))
			}
			actionTags := action.GetStringSlice("tags")
			actionQuestions := action.GetStringSlice("questions")
			query := strings.TrimSpace(action.GetString("search_query"))
			loop.Set("search_query", query)
			log.Infof("intent loop: searching capabilities with query: %s", query)
			keywords := strings.Split(query, " ")
			var results strings.Builder
			results.WriteString(fmt.Sprintf("## Capability Search Results for: %s\n\n", query))

			var capDetails []capabilityDetail

			// Include catalog pre-matched identifiers as supplementary context
			catalogMatched := loop.Get("catalog_matched_identifiers")
			if catalogMatched != "" {
				results.WriteString(fmt.Sprintf("### Pre-matched from Capability Catalog\n%s\n\n", catalogMatched))
			}

			// 1. Search AIYakTool via BM25 trigram - dual mode: AND + OR
			db := consts.GetGormProfileDatabase()
			if db != nil {
				loop.LoadingStatus("Start to load bm25+keyword search results for tools and AI forges... / 开始加载工具和AI蓝图的BM25+关键词搜索结果...")

				toolSeen := make(map[string]bool)
				var allTools []*schema.AIYakTool

				// just use OR search, bm25 will rank result
				tools, err := yakit.SearchAIYakToolBM25(db, &yakit.AIYakToolFilter{
					Keywords: keywords,
				}, 10, 0)
				if err != nil {
					log.Warnf("intent loop: BM25 tool AND-search failed: %v", err)
				}
				for _, t := range tools {
					if !toolSeen[t.Name] {
						toolSeen[t.Name] = true
						allTools = append(allTools, t)
					}
				}

				if len(allTools) > 0 {
					results.WriteString("### Matched Tools\n")
					for _, tool := range allTools {
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
						appendCapDetail(&capDetails, tool.Name, "tool", utils.ShrinkString(tool.Description, 200))
					}
					results.WriteString("\n")
					log.Infof("intent loop: found %d tools via BM25 (AND+OR)", len(allTools))

					var toolNames []string
					for _, t := range allTools {
						toolNames = append(toolNames, t.Name)
					}
					loop.Set("matched_tool_names", strings.Join(toolNames, ","))
				} else {
					results.WriteString("### Tools\nNo matching tools found.\n\n")
				}

				// 1.5 Search Yakit Plugins (YakScript) with enable_for_ai=true via BM25
				yakScripts, err := yakit.SearchYakScriptForAIBM25(db, &yakit.YakScriptForAIFilter{Keywords: keywords}, 10, 0)
				if err != nil {
					log.Warnf("intent loop: yakit plugin search failed: %v", err)
				}
				if len(yakScripts) > 0 {
					results.WriteString("### Matched Yakit Plugins\n")
					results.WriteString("These are Yakit plugins that can be loaded and executed. Use ScriptName (plugin ID) to load them.\n\n")
					var pluginNames []string
					for _, script := range yakScripts {
						pluginType := strings.ToUpper(script.Type)
						desc := script.AIDesc
						if desc == "" {
							desc = script.Help
						}
						desc = utils.ShrinkString(desc, 200)
						results.WriteString(fmt.Sprintf("- **[%s] %s**: %s", pluginType, script.ScriptName, desc))
						if script.AIKeywords != "" {
							results.WriteString(fmt.Sprintf(" [keywords: %s]", script.AIKeywords))
						}
						results.WriteString("\n")
						appendCapDetail(&capDetails, script.ScriptName, "yakit_plugin_"+script.Type, desc)
						pluginNames = append(pluginNames, script.ScriptName)
					}
					results.WriteString("\n")
					log.Infof("intent loop: found %d yakit plugins for AI", len(yakScripts))

					existingToolNames := loop.Get("matched_tool_names")
					if existingToolNames != "" {
						existingToolNames += ","
					}
					loop.Set("matched_tool_names", existingToolNames+strings.Join(pluginNames, ","))
				}

				// 2. Search AI Forges via BM25 trigram
				forgeSeen := make(map[string]bool)
				var allForges []*schema.AIForge

				forges, err := yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{
					ForgeTypes: schema.RunnableForgeTypes(),
					Keywords:   keywords,
				}, 10, 0)
				if err != nil {
					log.Warnf("intent loop: BM25 forge AND-search failed: %v", err)
				}
				for _, f := range forges {
					if !forgeSeen[f.ForgeName] {
						forgeSeen[f.ForgeName] = true
						allForges = append(allForges, f)
					}
				}

				if len(allForges) > 0 {
					results.WriteString("### Matched AI Forges (Blueprints)\n")
					for _, forge := range allForges {
						name := forge.ForgeName
						if forge.ForgeVerboseName != "" {
							name = forge.ForgeVerboseName + " (" + forge.ForgeName + ")"
						}
						desc := utils.ShrinkString(forge.Description, 200)
						results.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
						appendCapDetail(&capDetails, forge.ForgeName, "forge", utils.ShrinkString(forge.Description, 200))
					}
					results.WriteString("\n")
					log.Infof("intent loop: found %d forges (AND+OR)", len(allForges))

					var forgeNames []string
					for _, f := range allForges {
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
			searchSkillsFromLoader(r, query, &results, loop, &capDetails)

			// 4. Search registered loop metadata
			matchedLoops := searchLoopMetadata(query)
			if len(matchedLoops) > 0 {
				results.WriteString("### Matched Focus Modes\n")
				for _, meta := range matchedLoops {
					results.WriteString(fmt.Sprintf("- **%s**: %s\n", meta.Name, meta.Description))
					appendCapDetail(&capDetails, meta.Name, "focus_mode", meta.Description)
				}
				results.WriteString("\n")
				log.Infof("intent loop: found %d matching focus modes", len(matchedLoops))

				var loopNames []string
				for _, l := range matchedLoops {
					loopNames = append(loopNames, l.Name)
				}
				loop.Set("matched_loop_names", strings.Join(loopNames, ","))
			} else {
				availableModes := loop.Get("available_focus_modes")
				if availableModes != "" {
					results.WriteString("### Available Focus Modes (no direct match)\n")
					results.WriteString(availableModes)
					results.WriteString("\n")
				}
			}

			// Store structured capability details for finalize_enrichment to use
			if jsonStr := marshalCapabilityDetails(capDetails); jsonStr != "" {
				existingJSON := loop.Get("matched_capabilities_details")
				if existingJSON != "" {
					existing := parseCapabilityDetails(existingJSON)
					capDetails = append(existing, capDetails...)
					jsonStr = marshalCapabilityDetails(capDetails)
				}
				loop.Set("matched_capabilities_details", jsonStr)
			}

			// Store search results
			existingResults := loop.Get("search_results")
			if existingResults != "" {
				existingResults += "\n---\n\n"
			}
			loop.Set("search_results", existingResults+results.String())
			setTaskRetrievalInfo(loop,
				actionTags,
				actionQuestions,
				reactloops.CompactIntentSummary(loop.Get("intent_summary")),
			)

			op.Feedback(results.String())
			op.Continue()
		},
	)
}

// searchSkillsFromLoader searches skills via the SkillLoader if available in the runtime config.
// Uses type assertion to access GetSkillLoader() from the concrete Config type.
func searchSkillsFromLoader(r aicommon.AIInvokeRuntime, query string, results *strings.Builder, loop *reactloops.ReActLoop, capDetails *[]capabilityDetail) {
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

	matchedSkills, err := aiskillloader.SearchSkillMetas(skillLoader, query, 10)
	if err != nil {
		log.Warnf("intent loop: skill search failed: %v", err)
		return
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
			appendCapDetail(capDetails, skill.Name, "skill", desc)
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
