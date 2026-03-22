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

const NeedRecommendCapabilitiesCount = 15
const intentSummaryMaxRunes = reactloops.IntentSummaryMaxRunes

func VerifyIntentLoopNeedReAnalysis(loop *reactloops.ReActLoop) bool {
	existingAnalysis := loop.Get("intent_analysis")
	if existingAnalysis != "" {
		return false
	}
	existingSummary := loop.Get("intent_summary")
	if existingSummary != "" {
		matchedToolCount := len(strings.Split(loop.Get("matched_tool_names"), ","))
		matchedForgeCount := len(strings.Split(loop.Get("matched_forge_names"), ","))
		matchedSkillCounts := len(strings.Split(loop.Get("matched_skill_names"), ","))
		return matchedToolCount+matchedForgeCount+matchedSkillCounts > NeedRecommendCapabilitiesCount
	}
	return true
}

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
			if VerifyIntentLoopNeedReAnalysis(loop) {
				if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
					log.Infof("intent recognition loop ended due to max iterations, force generating summary via LiteForge")
				} else {
					log.Infof("intent recognition loop ended without finalize_enrichment (reason: %v), force generating summary", reason)
				}
				generateIntentSummaryViaLiteForge(loop, invoker)
			}

			logFinalIntentSummary(loop, invoker)
		}
	})
}

// generateIntentSummaryViaLiteForge uses InvokeLiteForge to generate a final intent
// analysis from collected search results when the loop exits without finalize_enrichment.
func generateIntentSummaryViaLiteForge(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	userQuery := loop.Get("user_query")
	searchResults := loop.Get("search_results")
	matchedToolNames := loop.Get("matched_tool_names")
	matchedForgeNames := loop.Get("matched_forge_names")
	matchedSkillNames := loop.Get("matched_skill_names")

	if searchResults == "" && matchedToolNames == "" && matchedForgeNames == "" && matchedSkillNames == "" {
		loop.Set("intent_summary", "未识别到可用能力")
		loop.Set("intent_analysis", "未识别到可用能力")
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
1. Summarize only the core user intent
2. intent_summary must be very short, ideally around 20 Chinese characters or similarly brief in English
3. Do NOT repeat the user's full request
4. Do NOT describe the search process
5. Do NOT include tools, forges, skills, or focus modes in intent_summary; put them only in recommended_capabilities
6. Output should be concise and actionable
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

	forgeResult, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx,
		"intent-finalize-summary",
		materials,
		[]aitool.ToolOption{
			aitool.WithStringParam("intent_summary",
				aitool.WithParam_Description("Concise structured summary of user intent following the template"),
				aitool.WithParam_Required(true)),
			aitool.WithStringArrayParamEx("recommended_capabilities",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("List of recommended capability names"),
				},
			),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "intent_summary"),
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "recommended_capabilities"),
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
	intentSummary = reactloops.CompactIntentSummary(intentSummary)
	recommendedCaps := VerifyIdentifiers(loop, forgeResult.GetStringSlice("recommended_capabilities"))

	// Merge catalog pre-matched identifiers
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

	// Build and set intent_analysis
	if intentSummary == "" {
		intentSummary = reactloops.CompactIntentSummary(userQuery)
	}
	loop.Set("intent_summary", intentSummary)
	loop.Set("intent_analysis", intentSummary)

	// Build tool recommendations from matched names + AI recommendations
	var toolRecommendations strings.Builder
	if matchedToolNames != "" {
		toolRecommendations.WriteString("Matched tools: " + matchedToolNames)
	}
	if len(recommendedCaps) > 0 {
		if toolRecommendations.Len() > 0 {
			toolRecommendations.WriteString("\n")
		}
		toolRecommendations.WriteString("AI recommended: " + strings.Join(recommendedCaps, ", "))
	}
	loop.Set("recommended_tools", toolRecommendations.String())

	if matchedForgeNames != "" {
		loop.Set("recommended_forges", "Matched forges: "+matchedForgeNames)
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
	if searchResults != "" {
		enrichment.WriteString("### Capability Search Results\n\n")
		enrichment.WriteString(searchResults)
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

	intentSummary := reactloops.CompactIntentSummary(loop.Get("intent_summary"))
	if intentSummary == "" {
		switch {
		case matchedToolNames != "" && matchedForgeNames != "":
			intentSummary = "识别相关工具与蓝图"
		case matchedToolNames != "":
			intentSummary = "识别相关工具能力"
		case matchedForgeNames != "":
			intentSummary = "识别相关蓝图能力"
		case matchedSkillNames != "":
			intentSummary = "识别相关技能能力"
		default:
			intentSummary = "意图识别已完成"
		}
	}
	loop.Set("intent_summary", intentSummary)
	loop.Set("intent_analysis", intentSummary)

	if matchedToolNames != "" {
		loop.Set("recommended_tools", "Matched tools: "+matchedToolNames)
	}
	if matchedForgeNames != "" {
		loop.Set("recommended_forges", "Matched forges: "+matchedForgeNames)
	}

	// Build structured capability enrichment from details if available
	capDetailsJSON := loop.Get("matched_capabilities_details")
	capDetails := parseCapabilityDetails(capDetailsJSON)
	var enrichment strings.Builder
	capMd := buildCapabilityEnrichmentMarkdown(capDetails, nil)
	if capMd != "" {
		enrichment.WriteString(capMd)
	}
	if searchResults != "" {
		enrichment.WriteString("### Capability Search Results\n\n")
		enrichment.WriteString(searchResults)
	}
	if enrichment.Len() > 0 {
		loop.Set("context_enrichment", enrichment.String())
	}

	log.Infof("intent finalize: using fallback analysis from raw search results")
}

// logFinalIntentSummary logs the final intent recognition summary.
func logFinalIntentSummary(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	intentAnalysis := loop.Get("intent_analysis")
	recommendedTools := loop.Get("recommended_tools")
	recommendedForges := loop.Get("recommended_forges")
	matchedSkillNames := loop.Get("matched_skill_names")

	var summary strings.Builder
	summary.WriteString("=== Intent Recognition Final Summary ===\n")

	compactSummary := reactloops.CompactIntentSummary(loop.Get("intent_summary"))
	if compactSummary == "" {
		compactSummary = reactloops.CompactIntentSummary(intentAnalysis)
	}
	if intentAnalysis != "" {
		summary.WriteString(fmt.Sprintf("[Intent] %s\n", compactSummary))
	}
	if recommendedTools != "" {
		summary.WriteString(fmt.Sprintf("[Tools] %s\n", utils.ShrinkString(recommendedTools, 120)))
	}
	if recommendedForges != "" {
		summary.WriteString(fmt.Sprintf("[Forges] %s\n", utils.ShrinkString(recommendedForges, 120)))
	}
	if matchedSkillNames != "" {
		summary.WriteString(fmt.Sprintf("[Skills] %s\n", utils.ShrinkString(matchedSkillNames, 120)))
	}

	summary.WriteString("=== End Intent Recognition Final Summary ===")

	log.Infof("%s", summary.String())

	if compactSummary != "" {
		invoker.AddToTimeline("intent_recognition_finalized", fmt.Sprintf("意图识别完成：%s", compactSummary))
	}
}

func compactIntentSummary(summary string) string {
	return reactloops.CompactIntentSummary(summary)
}
