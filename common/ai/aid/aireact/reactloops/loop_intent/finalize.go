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

// BuildOnPostIterationHook creates the post-iteration hook that ensures finalization
// always runs regardless of how the loop exits (normal exit, max iterations, or error).
//
// Pattern follows loop_knowledge_enhance: on loop done, check whether finalize_enrichment
// was called. If not (e.g. max iterations reached), force-generate intent summary via
// LiteForge. In all cases, log the final summary.
func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		// Always ignore errors so intent loop does not break the main loop
		operator.IgnoreError()

		if isDone {
			log.Infof("intent recognition loop done at iteration %d", iteration)

			// Check whether finalize_enrichment already set intent_analysis
			existingAnalysis := loop.Get("intent_analysis")
			if existingAnalysis == "" {
				// finalize_enrichment was NOT called (likely max iterations reached)
				if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
					log.Infof("intent recognition loop ended due to max iterations, force generating summary via LiteForge")
				} else {
					log.Infof("intent recognition loop ended without finalize_enrichment (reason: %v), force generating summary", reason)
				}
				generateIntentSummaryViaLiteForge(loop, invoker)
			}

			// Always log the final summary
			logFinalIntentSummary(loop, invoker)
		}
	})
}

// generateIntentSummaryViaLiteForge uses InvokeLiteForge to generate a final intent
// analysis from collected search results when the loop exits without finalize_enrichment.
// This mirrors how loop_knowledge_enhance generates reports on abnormal exits.
func generateIntentSummaryViaLiteForge(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	userQuery := loop.Get("user_query")
	searchResults := loop.Get("search_results")
	matchedToolNames := loop.Get("matched_tool_names")
	matchedForgeNames := loop.Get("matched_forge_names")
	matchedSkillNames := loop.Get("matched_skill_names")

	// If absolutely no data was collected, set a minimal analysis
	if searchResults == "" && matchedToolNames == "" && matchedForgeNames == "" && matchedSkillNames == "" {
		loop.Set("intent_analysis", "## Intent Analysis\n\nNo capabilities found for the given query.")
		log.Infof("intent finalize: no search results available, setting default analysis")
		return
	}

	ctx := loop.GetConfig().GetContext()
	nonce := utils.RandStringBytes(8)

	promptTemplate := `<|INSTRUCTION_{{ .Nonce }}|>
You are an intent analysis expert. The intent recognition loop has ended (reached iteration limit)
before the AI could produce a final analysis. Based on the search results collected so far,
generate a concise intent analysis for the user's query.

Requirements:
1. Summarize what the user wants to accomplish
2. List the matched capabilities (tools, forges, skills) and briefly explain why they are relevant
3. Provide recommended capabilities for the main loop to use
4. Output should be concise and actionable
<|INSTRUCTION_END_{{ .Nonce }}|>

<|USER_QUERY_{{ .Nonce }}|>
{{ .UserQuery }}
<|USER_QUERY_END_{{ .Nonce }}|>

<|SEARCH_RESULTS_{{ .Nonce }}|>
{{ .SearchResults }}
<|SEARCH_RESULTS_END_{{ .Nonce }}|>

<|MATCHED_CAPABILITIES_{{ .Nonce }}|>
Tools: {{ .MatchedToolNames }}
Forges: {{ .MatchedForgeNames }}
Skills: {{ .MatchedSkillNames }}
<|MATCHED_CAPABILITIES_END_{{ .Nonce }}|>`

	materials, err := utils.RenderTemplate(promptTemplate, map[string]any{
		"Nonce":             nonce,
		"UserQuery":         userQuery,
		"SearchResults":     utils.ShrinkString(searchResults, 4096),
		"MatchedToolNames":  matchedToolNames,
		"MatchedForgeNames": matchedForgeNames,
		"MatchedSkillNames": matchedSkillNames,
	})
	if err != nil {
		log.Errorf("intent finalize: template render failed: %v", err)
		buildFallbackIntentAnalysis(loop)
		return
	}

	forgeResult, err := invoker.InvokeLiteForge(
		ctx,
		"intent-finalize-summary",
		materials,
		[]aitool.ToolOption{
			aitool.WithStringParam("intent_summary",
				aitool.WithParam_Description("Concise summary of user intent with sub-goals"),
				aitool.WithParam_Required(true)),
			aitool.WithStringParam("recommended_capabilities",
				aitool.WithParam_Description("Comma-separated recommended tool/forge/skill names")),
			aitool.WithStringParam("context_notes",
				aitool.WithParam_Description("Additional context notes for the main loop")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "intent_summary"),
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "recommended_capabilities"),
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "context_notes"),
	)
	if err != nil {
		log.Errorf("intent finalize: LiteForge invocation failed: %v", err)
		buildFallbackIntentAnalysis(loop)
		return
	}
	if forgeResult == nil {
		log.Warnf("intent finalize: LiteForge returned nil result")
		buildFallbackIntentAnalysis(loop)
		return
	}

	intentSummary := strings.TrimSpace(forgeResult.GetString("intent_summary"))
	recommendedCapabilities := strings.TrimSpace(forgeResult.GetString("recommended_capabilities"))
	contextNotes := strings.TrimSpace(forgeResult.GetString("context_notes"))

	// Build and set intent_analysis
	var analysis strings.Builder
	analysis.WriteString("## Intent Analysis (auto-generated on loop exit)\n\n")
	if intentSummary != "" {
		analysis.WriteString(intentSummary)
	} else {
		analysis.WriteString("Unable to generate intent summary from collected data.")
	}
	analysis.WriteString("\n\n")
	loop.Set("intent_analysis", analysis.String())

	// Build tool recommendations from matched names + AI recommendations
	var toolRecommendations strings.Builder
	if matchedToolNames != "" {
		toolRecommendations.WriteString("Matched tools: " + matchedToolNames)
	}
	if recommendedCapabilities != "" {
		if toolRecommendations.Len() > 0 {
			toolRecommendations.WriteString("\n")
		}
		toolRecommendations.WriteString("AI recommended: " + recommendedCapabilities)
	}
	loop.Set("recommended_tools", toolRecommendations.String())

	// Build forge recommendations
	if matchedForgeNames != "" {
		loop.Set("recommended_forges", "Matched forges: "+matchedForgeNames)
	}

	// Build context enrichment
	var enrichment strings.Builder
	if searchResults != "" {
		enrichment.WriteString("### Capability Search Results\n\n")
		enrichment.WriteString(searchResults)
		enrichment.WriteString("\n")
	}
	if contextNotes != "" {
		enrichment.WriteString("### Additional Context\n\n")
		enrichment.WriteString(contextNotes)
		enrichment.WriteString("\n")
	}
	loop.Set("context_enrichment", enrichment.String())

	log.Infof("intent finalize: LiteForge generated summary successfully, intent_summary=%d bytes", len(intentSummary))
}

// buildFallbackIntentAnalysis creates a basic intent analysis from raw search results
// when LiteForge is not available or fails. Ensures loop variables are always populated.
func buildFallbackIntentAnalysis(loop *reactloops.ReActLoop) {
	searchResults := loop.Get("search_results")
	matchedToolNames := loop.Get("matched_tool_names")
	matchedForgeNames := loop.Get("matched_forge_names")
	matchedSkillNames := loop.Get("matched_skill_names")

	var analysis strings.Builder
	analysis.WriteString("## Intent Analysis (fallback - max iterations reached)\n\n")
	analysis.WriteString("The intent recognition loop reached its maximum iteration limit before producing a final analysis.\n\n")

	if matchedToolNames != "" {
		analysis.WriteString("### Matched Tools\n" + matchedToolNames + "\n\n")
	}
	if matchedForgeNames != "" {
		analysis.WriteString("### Matched Forges\n" + matchedForgeNames + "\n\n")
	}
	if matchedSkillNames != "" {
		analysis.WriteString("### Matched Skills\n" + matchedSkillNames + "\n\n")
	}

	loop.Set("intent_analysis", analysis.String())

	// Set recommendations based on raw matched names
	if matchedToolNames != "" {
		loop.Set("recommended_tools", "Matched tools: "+matchedToolNames)
	}
	if matchedForgeNames != "" {
		loop.Set("recommended_forges", "Matched forges: "+matchedForgeNames)
	}
	if searchResults != "" {
		loop.Set("context_enrichment", "### Capability Search Results\n\n"+searchResults)
	}

	log.Infof("intent finalize: using fallback analysis from raw search results")
}

// logFinalIntentSummary logs the final intent recognition summary.
// This is always called when the loop exits, ensuring the summary is visible in logs.
func logFinalIntentSummary(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	intentAnalysis := loop.Get("intent_analysis")
	recommendedTools := loop.Get("recommended_tools")
	recommendedForges := loop.Get("recommended_forges")
	contextEnrichment := loop.Get("context_enrichment")
	matchedSkillNames := loop.Get("matched_skill_names")

	var summary strings.Builder
	summary.WriteString("=== Intent Recognition Final Summary ===\n")

	if intentAnalysis != "" {
		summary.WriteString(fmt.Sprintf("[Intent Analysis] %s\n", utils.ShrinkString(intentAnalysis, 500)))
	}
	if recommendedTools != "" {
		summary.WriteString(fmt.Sprintf("[Recommended Tools] %s\n", recommendedTools))
	}
	if recommendedForges != "" {
		summary.WriteString(fmt.Sprintf("[Recommended Forges] %s\n", recommendedForges))
	}
	if matchedSkillNames != "" {
		summary.WriteString(fmt.Sprintf("[Matched Skills] %s\n", matchedSkillNames))
	}
	if contextEnrichment != "" {
		summary.WriteString(fmt.Sprintf("[Context Enrichment] %s\n", utils.ShrinkString(contextEnrichment, 500)))
	}

	summary.WriteString("=== End Intent Recognition Final Summary ===")

	log.Infof("%s", summary.String())

	invoker.AddToTimeline("intent_recognition_finalized", summary.String())
}
