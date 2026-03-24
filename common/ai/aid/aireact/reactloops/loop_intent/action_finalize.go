package loop_intent

import (
	"fmt"
	"io"
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
				"简短意图标签，只描述用户想做什么，优先保持完整语义。可控制在 20-24 字左右，但不要为了变短而截断关键含义。不要复述原请求，不要写推荐能力，不要解释搜索过程。/ Short intent label only, preferably around 20-24 Chinese characters while preserving complete meaning. Do not repeat the original request, include recommendations, or explain the search process."),
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
			{AINodeId: "intent", FieldName: "intent_summary", StreamHandler: intentSummaryStreamHandler},
			{AINodeId: "intent", FieldName: "recommended_capabilities", StreamHandler: recommendedCapabilitiesStreamHandler},
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
			intentSummary := reactloops.CompactIntentSummary(action.GetString("intent_summary"))
			recommendedCaps := action.GetStringSlice("recommended_capabilities")

			// Verify AI-recommended identifiers and merge with catalog pre-matched ones
			recommendedCaps = VerifyIdentifiers(loop, recommendedCaps)

			catalogMatched := loop.Get("catalog_matched_identifiers")
			if catalogMatched != "" {
				seen := make(map[string]bool)
				for _, c := range recommendedCaps {
					seen[c] = true
				}
				for _, id := range strings.Split(catalogMatched, ",") {
					id = strings.TrimSpace(id)
					if id != "" && !seen[id] {
						recommendedCaps = append(recommendedCaps, id)
						seen[id] = true
					}
				}
			}

			log.Infof("intent loop: finalizing enrichment - summary: %s, verified caps: %v",
				utils.ShrinkString(intentSummary, 200), recommendedCaps)

			// Build the structured intent analysis
			loop.Set("intent_summary", intentSummary)

			var analysis strings.Builder
			analysis.WriteString(intentSummary)

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

			r.AddToTimeline("intent_finalized", fmt.Sprintf("意图识别完成：%s", intentSummary))

			log.Infof("intent loop: enrichment finalized, exiting loop")
			op.Exit()
		},
	)
}

func intentSummaryStreamHandler(fieldReader io.Reader, emitWriter io.Writer) {
	content, err := io.ReadAll(fieldReader)
	if err != nil {
		return
	}
	_, _ = emitWriter.Write([]byte(reactloops.CompactIntentSummary(string(content))))
}
