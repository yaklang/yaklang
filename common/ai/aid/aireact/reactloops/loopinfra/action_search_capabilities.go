package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_SearchCapabilities = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_SEARCH_CAPABILITIES,
	Description: "Search for available capabilities (tools, AI forges/blueprints, skills, focus modes) " +
		"by running a deep intent recognition loop. Use this when the current tool list is insufficient " +
		"or you need to discover specialized capabilities for the task at hand.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"search_query",
			aitool.WithParam_Description(
				"Describe what capabilities you need using natural language, "+
					"e.g. 'port scanning vulnerability detection', 'encode base64 codec', 'report generation'. "+
					"The system will run an intent recognition loop to discover matching tools, forges, skills, and focus modes."),
			aitool.WithParam_Required(true),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "search_query", AINodeId: "search_capabilities"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		query := strings.TrimSpace(action.GetString("search_query"))
		if query == "" {
			return utils.Error("search_query is required for search_capabilities action but empty")
		}
		loop.Set("_search_capabilities_query", query)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		query := loop.Get("_search_capabilities_query")
		if query == "" {
			operator.Feedback("search_query is empty, cannot search capabilities")
			operator.Continue()
			return
		}

		invoker := loop.GetInvoker()

		log.Infof("search_capabilities action: running capability search for query: %s", utils.ShrinkString(query, 200))
		invoker.AddToTimeline("search_capabilities_start", fmt.Sprintf("开始搜索能力：%s", reactloops.CompactIntentSummary(query)))

		searchResult, err := reactloops.SearchCapabilities(invoker, loop, reactloops.CapabilitySearchInput{
			Query:               query,
			IncludeCatalogMatch: true,
		})
		if err != nil {
			log.Warnf("search_capabilities action: capability search failed: %v", err)
			operator.Feedback(fmt.Sprintf("Capability search failed: %v. Try a different query or proceed with currently available tools.", err))
			operator.Continue()
			return
		}
		if searchResult == nil {
			operator.Feedback("Capability search completed but no results could be extracted.")
			operator.Continue()
			return
		}
		reactloops.ApplyCapabilitySearchResult(invoker, loop, searchResult)

		compactIntent := reactloops.CompactIntentSummary(query)
		loop.Set("intent_analysis", compactIntent)
		if recommendedTools := renderCapabilityToolRecommendations(searchResult); recommendedTools != "" {
			loop.Set("intent_recommended_tools", recommendedTools)
		}
		if recommendedForges := renderCapabilityForgeRecommendations(searchResult); recommendedForges != "" {
			loop.Set("intent_recommended_forges", recommendedForges)
		}
		if searchResult != nil && searchResult.ContextEnrichment != "" {
			loop.Set("intent_context_enrichment", searchResult.ContextEnrichment)
		}

		matchedToolNames := strings.Join(searchResult.MatchedToolNames, ",")
		matchedForgeNames := strings.Join(searchResult.MatchedForgeNames, ",")
		matchedSkillNames := strings.Join(searchResult.MatchedSkillNames, ",")

		log.Infof("search_capabilities action: capability search completed, tools=%s, forges=%s, skills=%s",
			matchedToolNames, matchedForgeNames, matchedSkillNames)

		var summary strings.Builder
		summary.WriteString("能力搜索已完成\n")
		if compactIntent != "" {
			summary.WriteString("意图：" + compactIntent + "\n")
		}
		if tools := reactloops.CompactCapabilityNames(matchedToolNames, 3); tools != "" {
			summary.WriteString("工具：" + tools + "\n")
		}
		if forges := reactloops.CompactCapabilityNames(matchedForgeNames, 3); forges != "" {
			summary.WriteString("蓝图：" + forges + "\n")
		}
		if skills := reactloops.CompactCapabilityNames(matchedSkillNames, 3); skills != "" {
			summary.WriteString("技能：" + skills + "\n")
		}
		summary.WriteString("相关能力已加入上下文，可继续执行任务。")

		invoker.AddToTimeline("search_capabilities_completed",
			fmt.Sprintf("能力搜索完成：%s | 工具[%s] 蓝图[%s] 技能[%s]",
				compactIntent,
				reactloops.CompactCapabilityNames(matchedToolNames, 2),
				reactloops.CompactCapabilityNames(matchedForgeNames, 2),
				reactloops.CompactCapabilityNames(matchedSkillNames, 2)))

		operator.Feedback(summary.String())
		operator.Continue()
	},
}

func renderCapabilityToolRecommendations(result *reactloops.CapabilitySearchResult) string {
	if result == nil {
		return ""
	}
	var builder strings.Builder
	if len(result.MatchedToolNames) > 0 {
		builder.WriteString("匹配工具 / Matched tools: " + strings.Join(result.MatchedToolNames, ","))
	}
	if len(result.MatchedSkillNames) > 0 {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString("匹配技能 / Matched skills: " + strings.Join(result.MatchedSkillNames, ","))
	}
	if len(result.RecommendedCapabilities) > 0 {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString("推荐能力 / Recommended capabilities: " + strings.Join(result.RecommendedCapabilities, ","))
	}
	return builder.String()
}

func renderCapabilityForgeRecommendations(result *reactloops.CapabilitySearchResult) string {
	if result == nil || len(result.MatchedForgeNames) == 0 {
		return ""
	}
	return "匹配蓝图 / Matched forges: " + strings.Join(result.MatchedForgeNames, ",")
}

func populateExtraCapabilitiesFromIntent(
	invoker aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	matchedToolNames, matchedForgeNames, matchedSkillNames string,
) {
	ecm := loop.GetExtraCapabilities()
	if ecm == nil {
		return
	}

	cfg := invoker.GetConfig()

	if matchedToolNames != "" {
		toolNames := splitAndTrimNames(matchedToolNames)
		toolMgr := cfg.GetAiToolManager()
		if toolMgr != nil {
			for _, name := range toolNames {
				tool, err := toolMgr.GetToolByName(name)
				if err != nil {
					log.Debugf("search_capabilities: skip tool %q: %v", name, err)
					continue
				}
				ecm.AddTools(tool)
			}
		}
	}

	if matchedForgeNames != "" {
		forgeNames := splitAndTrimNames(matchedForgeNames)
		type forgeManagerProvider interface {
			GetAIForgeManager() aicommon.AIForgeFactory
		}
		if provider, ok := cfg.(forgeManagerProvider); ok {
			forgeMgr := provider.GetAIForgeManager()
			if forgeMgr != nil {
				for _, name := range forgeNames {
					forge, err := forgeMgr.GetAIForge(name)
					if err != nil {
						log.Debugf("search_capabilities: skip forge %q: %v", name, err)
						continue
					}
					ecm.AddForges(reactloops.ExtraForgeInfo{
						Name:        forge.ForgeName,
						VerboseName: forge.ForgeVerboseName,
						Description: forge.Description,
					})
				}
			}
		}
	}

	if matchedSkillNames != "" {
		skillNames := splitAndTrimNames(matchedSkillNames)
		type skillLoaderProvider interface {
			GetSkillLoader() aiskillloader.SkillLoader
		}
		if provider, ok := cfg.(skillLoaderProvider); ok {
			skillLoader := provider.GetSkillLoader()
			if skillLoader != nil && skillLoader.HasSkills() {
				for _, name := range skillNames {
					meta, err := aiskillloader.LookupSkillMeta(skillLoader, name)
					if err != nil || meta == nil {
						log.Debugf("search_capabilities: skip skill %q: %v", name, err)
						continue
					}
					ecm.AddSkills(reactloops.ExtraSkillInfo{
						Name:        meta.Name,
						Description: meta.Description,
					})
				}
			}
		}
	}

	if ecm.HasCapabilities() {
		log.Infof("search_capabilities action: extra capabilities populated: %d tools, %d forges, %d skills",
			ecm.ToolCount(), len(ecm.ListForges()), len(ecm.ListSkills()))
	}
}

func splitAndTrimNames(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
