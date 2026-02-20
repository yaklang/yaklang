package loop_intent

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// finalizeEnrichmentAction creates the finalize_enrichment action
// that produces the final intent analysis and context enrichment.
var finalizeEnrichmentAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeFinalizeEnrichmentAction(r)
}

func makeFinalizeEnrichmentAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "完成意图分析并生成上下文增强信息。在搜索能力之后调用，汇总发现并生成推荐。/ " +
		"Finalize the intent analysis and produce context enrichment after capability search."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("intent_summary",
			aitool.WithParam_Description(
				"结构化意图摘要，大致遵循模板，表达含义即可：「用户说「{摘要}」，目的是：{意图}。通过搜索「{关键词}」得到的结果，"+
					"可以推荐接下来的步骤使用工具 {tools}，蓝图（Forge）{forges}，加载技能 {skills}。"+
					"启动专注模式：{focus_modes} 来实现目标。」无匹配的类型可省略。/ "+
					"Structured intent summary following the template. Omit capability types with no matches."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringArrayParamEx("recommended_capabilities",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("推荐的能力名称列表（工具、蓝图、技能、专注模式的名称）。/ List of recommended capability names (tools, forges, skills, focus modes)."),
			},
		),
	}

	return reactloops.WithRegisterLoopActionWithStreamField(
		"finalize_enrichment",
		desc,
		toolOpts,
		[]*reactloops.LoopStreamField{
			{AINodeId: "intent", FieldName: "intent_summary"},
			{AINodeId: "intent", FieldName: "recommended_capabilities"},
		},
		// Verifier
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			summary := strings.TrimSpace(action.GetString("intent_summary"))
			if summary == "" {
				return utils.Error("intent_summary is required for finalization")
			}
			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			intentSummary := strings.TrimSpace(action.GetString("intent_summary"))
			recommendedCaps := action.GetStringSlice("recommended_capabilities")

			log.Infof("intent loop: finalizing enrichment - summary: %s", utils.ShrinkString(intentSummary, 200))

			// Build the structured intent analysis
			var analysis strings.Builder
			analysis.WriteString("## 意图分析 / Intent Analysis\n\n")
			analysis.WriteString(intentSummary)
			analysis.WriteString("\n\n")

			// Build recommended tools/forges from search + AI recommendations
			matchedToolNames := loop.Get("matched_tool_names")
			matchedForgeNames := loop.Get("matched_forge_names")
			matchedSkillNames := loop.Get("matched_skill_names")

			var toolRecommendations strings.Builder
			if matchedToolNames != "" {
				toolRecommendations.WriteString("匹配工具 / Matched tools: " + matchedToolNames)
			}
			if len(recommendedCaps) > 0 {
				if toolRecommendations.Len() > 0 {
					toolRecommendations.WriteString("\n")
				}
				toolRecommendations.WriteString("AI 推荐 / AI recommended: " + strings.Join(recommendedCaps, ", "))
			}

			var forgeRecommendations strings.Builder
			if matchedForgeNames != "" {
				forgeRecommendations.WriteString("匹配蓝图 / Matched forges: " + matchedForgeNames)
			}

			if matchedSkillNames != "" {
				if toolRecommendations.Len() > 0 {
					toolRecommendations.WriteString("\n")
				}
				toolRecommendations.WriteString("匹配技能 / Matched skills: " + matchedSkillNames)
			}

			// Build structured capability enrichment Markdown
			recSet := make(map[string]bool, len(recommendedCaps))
			for _, name := range recommendedCaps {
				recSet[strings.TrimSpace(name)] = true
			}

			capDetailsJSON := loop.Get("matched_capabilities_details")
			capDetails := parseCapabilityDetails(capDetailsJSON)

			var enrichment strings.Builder
			capMd := buildCapabilityEnrichmentMarkdown(capDetails, recSet)
			if capMd != "" {
				enrichment.WriteString(capMd)
			}
			searchResults := loop.Get("search_results")
			if searchResults != "" {
				enrichment.WriteString("### 能力搜索结果 / Capability Search Results\n\n")
				enrichment.WriteString(searchResults)
				enrichment.WriteString("\n")
			}

			// Store final results in loop variables for the caller to extract
			loop.Set("intent_analysis", analysis.String())
			loop.Set("recommended_tools", toolRecommendations.String())
			loop.Set("recommended_forges", forgeRecommendations.String())
			loop.Set("context_enrichment", enrichment.String())

			r.AddToTimeline("intent_finalized", fmt.Sprintf(
				"Intent analysis completed: %s | Tools: %s | Forges: %s | Skills: %s",
				utils.ShrinkString(intentSummary, 100),
				matchedToolNames,
				matchedForgeNames,
				matchedSkillNames,
			))

			log.Infof("intent loop: enrichment finalized, exiting loop")
			op.Exit()
		},
	)
}
