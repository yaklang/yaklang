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
			aitool.WithParam_Description("用户意图的简洁摘要，如有多个子目标请逐一列出。/ Concise summary of the user's intent, with sub-goals if applicable."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("recommended_capabilities",
			aitool.WithParam_Description("推荐的工具名、蓝图名或专注模式，逗号分隔。/ Comma-separated recommended tool/forge/focus mode names."),
		),
		aitool.WithStringParam("context_notes",
			aitool.WithParam_Description("帮助主循环更好处理用户请求的补充上下文或备注。/ Additional context to help the main loop handle the request."),
		),
	}

	return reactloops.WithRegisterLoopActionWithStreamField(
		"finalize_enrichment",
		desc,
		toolOpts,
		[]*reactloops.LoopStreamField{
			{AINodeId: "intent", FieldName: "intent_summary"},
			{AINodeId: "intent", FieldName: "recommended_capabilities"},
			{AINodeId: "intent", FieldName: "context_notes"},
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
			recommendedCapabilities := strings.TrimSpace(action.GetString("recommended_capabilities"))
			contextNotes := strings.TrimSpace(action.GetString("context_notes"))

			log.Infof("intent loop: finalizing enrichment - summary: %s", utils.ShrinkString(intentSummary, 200))

			// Build the structured intent analysis
			var analysis strings.Builder
			analysis.WriteString("## 意图分析 / Intent Analysis\n\n")
			analysis.WriteString(intentSummary)
			analysis.WriteString("\n\n")

			// Build recommended tools section from search results + AI recommendations
			var toolRecommendations strings.Builder
			matchedToolNames := loop.Get("matched_tool_names")
			if matchedToolNames != "" {
				toolRecommendations.WriteString("匹配工具 / Matched tools: " + matchedToolNames)
			}
			if recommendedCapabilities != "" {
				if toolRecommendations.Len() > 0 {
					toolRecommendations.WriteString("\n")
				}
				toolRecommendations.WriteString("AI 推荐 / AI recommended: " + recommendedCapabilities)
			}

			// Build forge recommendations
			var forgeRecommendations strings.Builder
			matchedForgeNames := loop.Get("matched_forge_names")
			if matchedForgeNames != "" {
				forgeRecommendations.WriteString("匹配蓝图 / Matched forges: " + matchedForgeNames)
			}

			// Build skill recommendations
			matchedSkillNames := loop.Get("matched_skill_names")
			if matchedSkillNames != "" {
				if toolRecommendations.Len() > 0 {
					toolRecommendations.WriteString("\n")
				}
				toolRecommendations.WriteString("匹配技能 / Matched skills: " + matchedSkillNames)
			}

			// Build enrichment context
			var enrichment strings.Builder
			searchResults := loop.Get("search_results")
			if searchResults != "" {
				enrichment.WriteString("### 能力搜索结果 / Capability Search Results\n\n")
				enrichment.WriteString(searchResults)
				enrichment.WriteString("\n")
			}
			if contextNotes != "" {
				enrichment.WriteString("### 补充上下文 / Additional Context\n\n")
				enrichment.WriteString(contextNotes)
				enrichment.WriteString("\n")
			}

			// Store final results in loop variables for the caller to extract
			loop.Set("intent_analysis", analysis.String())
			loop.Set("recommended_tools", toolRecommendations.String())
			loop.Set("recommended_forges", forgeRecommendations.String())
			loop.Set("context_enrichment", enrichment.String())

			// Add to timeline
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
