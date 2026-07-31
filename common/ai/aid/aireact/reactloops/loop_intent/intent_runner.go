package loop_intent

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/intent_prompt.txt
var intentPromptTpl string

//go:embed prompts/recommend_prompt.txt
var recommendPromptTpl string

// needRecommendThreshold controls when a second LiteForge AI call is made for
// capability recommendation. When the matched tool/forge/skill count exceeds
// this threshold, the search results are too noisy for the main loop to
// consume directly, so we ask the AI to pick the most relevant ones.
const needRecommendThreshold = 12

// intentKeywordResult holds the output of the single AI call for keyword + intent.
type intentKeywordResult struct {
	IntentSummary  string
	SearchKeywords []string // each element is a space-separated keyword group
	Tags           []string
	Questions      []string
}

// runIntentRecognition is the core of the simplified loop_intent.
// It performs at most 2 AI calls:
//  1. Generate intent_summary + search_keywords (always).
//  2. Recommend capabilities (only when matched results are too many).
// All local capability search (BM25, skills, focus modes) is done without AI.
func runIntentRecognition(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop) *reactloops.DeepIntentResult {
	totalStart := time.Now()
	ctx := loop.GetConfig().GetContext()
	userQuery := loop.Get("user_query")

	// ── Step 1: Single AI call — generate intent_summary + search_keywords ──
	keywordResult, err := generateIntentKeywords(r, ctx, userQuery)
	if err != nil {
		log.Warnf("intent loop: generateIntentKeywords failed: %v, falling back to user query", err)
		keywordResult = &intentKeywordResult{
			IntentSummary:  reactloops.CompactIntentSummary(userQuery),
			SearchKeywords: []string{userQuery},
		}
	}
	keywordResult.IntentSummary = reactloops.CompactIntentSummary(keywordResult.IntentSummary)

	loop.Set("intent_summary", keywordResult.IntentSummary)
	log.Infof("intent loop: intent_summary=%s, keyword_groups=%v", keywordResult.IntentSummary, keywordResult.SearchKeywords)

	// ── Step 2: Local capability search via BM25 (no AI) ──
	searchInput := reactloops.CapabilitySearchInput{
		Query:   userQuery,
		Queries: keywordResult.SearchKeywords,
		Limit:   10,
	}
	searchResult, err := reactloops.SearchCapabilities(r, loop, searchInput)
	if err != nil {
		log.Warnf("intent loop: SearchCapabilities failed: %v", err)
		searchResult = &reactloops.CapabilitySearchResult{}
	}

	// ── Step 3: Conditional AI recommendation (only when results are too many) ──
	totalMatched := len(searchResult.MatchedToolNames) + len(searchResult.MatchedForgeNames) + len(searchResult.MatchedSkillNames)
	recommended := searchResult.RecommendedCapabilities

	if totalMatched > needRecommendThreshold {
		log.Infof("intent loop: %d matched capabilities exceed threshold %d, invoking AI recommendation", totalMatched, needRecommendThreshold)
		rec, err := recommendCapabilities(r, ctx, userQuery, keywordResult.IntentSummary, searchResult)
		if err != nil {
			log.Warnf("intent loop: recommendCapabilities failed: %v, using all matched as recommended", err)
		} else if len(rec) > 0 {
			recommended = rec
		}
	}

	recommended = reactloops.ApplyScriptEditExecutionPolicy(loop, recommended)
	reactloops.PreloadSingleRecommendedTool(loop, recommended)

	// ── Step 4: Build enrichment markdown ──
	recSet := make(map[string]bool, len(recommended))
	for _, name := range recommended {
		recSet[strings.TrimSpace(name)] = true
	}
	contextEnrichment := reactloops.BuildCapabilityEnrichmentMarkdown(searchResult.Details, recSet)

	// ── Step 5: Set loop variables for deep_intent.go to extract ──
	loop.Set("intent_analysis", keywordResult.IntentSummary)
	loop.Set("recommended_tools", buildToolRecommendation(searchResult.MatchedToolNames, recommended))
	loop.Set("recommended_forges", buildForgeRecommendation(searchResult.MatchedForgeNames))
	loop.Set("context_enrichment", contextEnrichment)
	loop.Set("matched_tool_names", strings.Join(searchResult.MatchedToolNames, ","))
	loop.Set("matched_forge_names", strings.Join(searchResult.MatchedForgeNames, ","))
	loop.Set("matched_skill_names", strings.Join(searchResult.MatchedSkillNames, ","))

	// task retrieval info
	setTaskRetrievalTags(loop, keywordResult.Tags, keywordResult.Questions, keywordResult.IntentSummary)

	// ── Step 6: Apply skills to catalog ──
	applyMatchedSkills(loop, r, searchResult.MatchedSkillNames)

	log.Infof("intent loop: completed in %v — %d tools, %d forges, %d skills",
		time.Since(totalStart), len(searchResult.MatchedToolNames),
		len(searchResult.MatchedForgeNames), len(searchResult.MatchedSkillNames))

	return &reactloops.DeepIntentResult{
		IntentAnalysis:    keywordResult.IntentSummary,
		RecommendedTools:  loop.Get("recommended_tools"),
		RecommendedForges: loop.Get("recommended_forges"),
		ContextEnrichment: contextEnrichment,
		MatchedToolNames:  loop.Get("matched_tool_names"),
		MatchedForgeNames: loop.Get("matched_forge_names"),
		MatchedSkillNames: loop.Get("matched_skill_names"),
	}
}

// generateIntentKeywords performs the single AI call to produce intent_summary
// and search_keywords from user input.
func generateIntentKeywords(r aicommon.AIInvokeRuntime, ctx context.Context, userQuery string) (*intentKeywordResult, error) {
	nonce := utils.RandStringBytes(8)
	prompt, err := utils.RenderTemplate(intentPromptTpl, map[string]any{
		"Nonce":     nonce,
		"UserQuery": userQuery,
	})
	if err != nil {
		return nil, utils.Wrap(err, "render intent prompt failed")
	}

	outputs := []aitool.ToolOption{
		aitool.WithStringParam("intent_summary",
			aitool.WithParam_Description("Concise intent label, around 20-24 Chinese chars or similar English, preserving complete meaning. Do not repeat the request, list tools, or explain the search process."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringArrayParamEx("search_keywords",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Keyword groups for BM25 capability search. Each element is a space-separated keyword string (e.g. '端口扫描 port scan'). Use both Chinese and English. Multiple groups for composite tasks."),
			},
		),
		aitool.WithStringArrayParamEx("tags",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Task tags for downstream retrieval — domain tags, action tags, capability tags."),
			},
		),
		aitool.WithStringArrayParamEx("questions",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Key retrieval questions directly usable for knowledge retrieval."),
			},
		),
	}

	forgeResult, err := r.InvokeSpeedPriorityLiteForge(ctx, "intent-keyword-gen", prompt, outputs,
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "intent_summary"),
	)
	if err != nil {
		return nil, err
	}
	if forgeResult == nil {
		return nil, utils.Error("intent keyword generation returned nil result")
	}

	result := &intentKeywordResult{
		IntentSummary:  forgeResult.GetString("intent_summary"),
		SearchKeywords: forgeResult.GetStringSlice("search_keywords"),
		Tags:           forgeResult.GetStringSlice("tags"),
		Questions:      forgeResult.GetStringSlice("questions"),
	}

	if len(result.SearchKeywords) == 0 {
		result.SearchKeywords = []string{userQuery}
	}

	return result, nil
}

// recommendCapabilities performs the second (conditional) AI call to select the
// most relevant capabilities from the matched search results.
func recommendCapabilities(r aicommon.AIInvokeRuntime, ctx context.Context, userQuery, intentSummary string, searchResult *reactloops.CapabilitySearchResult) ([]string, error) {
	matchedCapList := buildMatchedCapabilitiesText(searchResult)
	if matchedCapList == "" {
		return nil, nil
	}

	nonce := utils.RandStringBytes(8)
	prompt, err := utils.RenderTemplate(recommendPromptTpl, map[string]any{
		"Nonce":               nonce,
		"UserQuery":           userQuery,
		"IntentSummary":       intentSummary,
		"MatchedCapabilities": matchedCapList,
	})
	if err != nil {
		return nil, utils.Wrap(err, "render recommend prompt failed")
	}

	outputs := []aitool.ToolOption{
		aitool.WithStringArrayParamEx("recommended_capabilities", []aitool.PropertyOption{
			aitool.WithParam_Description("List of matched capability identifiers that are most relevant to the user's intent. Only include identifiers from the matched list."),
			aitool.WithParam_Required(true),
		}),
	}

	forgeResult, err := r.InvokeSpeedPriorityLiteForge(ctx, "intent-capability-recommend", prompt, outputs)
	if err != nil {
		return nil, err
	}
	if forgeResult == nil {
		return nil, nil
	}

	caps := forgeResult.GetStringSlice("recommended_capabilities")
	return reactloops.NormalizeCapabilityNames(strings.Join(caps, ",")), nil
}

// buildMatchedCapabilitiesText formats the search result into a compact text list
// for the recommendation AI call.
func buildMatchedCapabilitiesText(result *reactloops.CapabilitySearchResult) string {
	if result == nil {
		return ""
	}
	var sb strings.Builder
	for _, d := range result.Details {
		sb.WriteString(fmt.Sprintf("[%s:%s]: %s\n", d.CapabilityType, d.CapabilityName, d.Description))
	}
	return strings.TrimSpace(sb.String())
}

// buildToolRecommendation combines matched tool names and AI recommended capabilities.
func buildToolRecommendation(matchedToolNames []string, recommended []string) string {
	var sb strings.Builder
	if len(matchedToolNames) > 0 {
		sb.WriteString("Matched tools: " + strings.Join(matchedToolNames, ", "))
	}
	if len(recommended) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("AI recommended: " + strings.Join(recommended, ", "))
	}
	return sb.String()
}

func buildForgeRecommendation(matchedForgeNames []string) string {
	if len(matchedForgeNames) == 0 {
		return ""
	}
	return "Matched forges: " + strings.Join(matchedForgeNames, ", ")
}

// setTaskRetrievalTags writes retrieval info into loop vars (consumed by deep_intent.go).
func setTaskRetrievalTags(loop *reactloops.ReActLoop, tags, questions []string, target string) {
	if loop == nil {
		return
	}
	target = strings.TrimSpace(target)
	if len(tags) > 0 {
		loop.Set("task_retrieval_tags", strings.Join(tags, "\n"))
	}
	if len(questions) > 0 {
		loop.Set("task_retrieval_questions", strings.Join(questions, "\n"))
	}
	if target != "" {
		loop.Set("task_retrieval_target", target)
	}
}

// applyMatchedSkills resolves matched skill names to SkillMeta objects and
// applies them to the catalog.
func applyMatchedSkills(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, matchedSkillNames []string) {
	if len(matchedSkillNames) == 0 {
		return
	}
	type skillLoaderProvider interface {
		GetSkillLoader() aiskillloader.SkillLoader
	}
	provider, ok := r.GetConfig().(skillLoaderProvider)
	if !ok {
		return
	}
	skillLoader := provider.GetSkillLoader()
	if skillLoader == nil || !skillLoader.HasSkills() {
		return
	}
	var matchedMetas []*aiskillloader.SkillMeta
	for _, name := range matchedSkillNames {
		meta, err := aiskillloader.LookupSkillMeta(skillLoader, name)
		if err != nil || meta == nil {
			continue
		}
		matchedMetas = append(matchedMetas, meta)
	}
	if len(matchedMetas) > 0 {
		reactloops.ApplyMatchedSkillsToCatalog(loop, r.GetConfig(), matchedMetas)
	}
}
