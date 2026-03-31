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
		cfg := invoker.GetConfig()
		ctx := cfg.GetContext()
		task := loop.GetCurrentTask()
		if task != nil {
			ctx = task.GetContext()
		}

		log.Infof("search_capabilities action: running intent loop for query: %s", utils.ShrinkString(query, 200))
		invoker.AddToTimeline("search_capabilities_start", fmt.Sprintf("开始搜索能力：%s", reactloops.CompactIntentSummary(query)))

		intentTask := aicommon.NewStatefulTaskBase(
			invoker.GetCurrentTaskId()+"_search_cap",
			query,
			ctx,
			cfg.GetEmitter(),
		)

		originOptions := cfg.OriginOptions()
		var opts []any
		for _, option := range originOptions {
			opts = append(opts, option)
		}

		var intentLoop *reactloops.ReActLoop
		opts = append(opts, reactloops.WithOnLoopInstanceCreated(func(l *reactloops.ReActLoop) {
			intentLoop = l
		}))

		_, err := invoker.ExecuteLoopTaskIF(schema.AI_REACT_LOOP_NAME_INTENT, intentTask, opts...)
		if err != nil {
			log.Warnf("search_capabilities action: intent loop failed: %v", err)
			operator.Feedback(fmt.Sprintf("Capability search failed: %v. Try a different query or proceed with currently available tools.", err))
			operator.Continue()
			return
		}

		if intentLoop == nil {
			log.Warnf("search_capabilities action: intent loop reference is nil")
			operator.Feedback("Capability search completed but no results could be extracted.")
			operator.Continue()
			return
		}

		intentAnalysis := intentLoop.Get("intent_analysis")
		recommendedTools := intentLoop.Get("recommended_tools")
		recommendedForges := intentLoop.Get("recommended_forges")
		contextEnrichment := intentLoop.Get("context_enrichment")
		retrievalTags := intentLoop.Get("task_retrieval_tags")
		retrievalQuestions := intentLoop.Get("task_retrieval_questions")
		retrievalTarget := intentLoop.Get("task_retrieval_target")
		matchedToolNames := intentLoop.Get("matched_tool_names")
		matchedForgeNames := intentLoop.Get("matched_forge_names")
		matchedSkillNames := intentLoop.Get("matched_skill_names")

		log.Infof("search_capabilities action: intent loop completed, analysis=%d bytes, tools=%s, forges=%s, skills=%s",
			len(intentAnalysis), matchedToolNames, matchedForgeNames, matchedSkillNames)

		compactIntent := reactloops.CompactIntentSummary(intentAnalysis)
		if compactIntent == "" {
			compactIntent = reactloops.CompactIntentSummary(query)
		}

		if intentAnalysis != "" {
			loop.Set("intent_analysis", compactIntent)
			invoker.AddToTimeline("search_capabilities_analysis", fmt.Sprintf("意图识别：%s", compactIntent))
		}
		if recommendedTools != "" {
			loop.Set("intent_recommended_tools", recommendedTools)
		}
		if recommendedForges != "" {
			loop.Set("intent_recommended_forges", recommendedForges)
		}
		if contextEnrichment != "" {
			loop.Set("intent_context_enrichment", contextEnrichment)
		}
		reactloops.ApplyTaskRetrievalInfoToTask(task, retrievalTags, retrievalQuestions, retrievalTarget)

		populateExtraCapabilitiesFromIntent(invoker, loop, matchedToolNames, matchedForgeNames, matchedSkillNames)

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
