package loop_default

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// InputScale represents the complexity level of user input.
// Used for fast input classification (<5ms) to decide
// whether to use fast-mode (rules + BM25) or deep intent recognition.
type InputScale int

const (
	InputScaleMicro  InputScale = iota // < 20 runes, single word/phrase
	InputScaleSmall                    // < 100 runes, simple sentence
	InputScaleMedium                   // 100-500 runes
	InputScaleLarge                    // 500-2000 runes
	InputScaleXLarge                   // > 2000 runes
)

func (s InputScale) String() string {
	switch s {
	case InputScaleMicro:
		return "Micro"
	case InputScaleSmall:
		return "Small"
	case InputScaleMedium:
		return "Medium"
	case InputScaleLarge:
		return "Large"
	case InputScaleXLarge:
		return "XLarge"
	default:
		return "Unknown"
	}
}

// IsMicroOrSmall returns true if the input is classified as Micro or Small,
// suitable for fast-mode intent matching.
func (s InputScale) IsMicroOrSmall() bool {
	return s <= InputScaleSmall
}

// ClassifyInputScale classifies user input into a scale level based on
// length, sentence count, and content complexity.
// This function is designed to execute in <5ms with no I/O or AI calls.
func ClassifyInputScale(input string) InputScale {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return InputScaleMicro
	}

	runeCount := utf8.RuneCountInString(trimmed)

	// Code blocks or structured content bump up the scale
	hasCodeBlock := strings.Contains(trimmed, "```")
	sentenceCount := countSentences(trimmed)
	hasURL := urlPattern.MatchString(trimmed)
	hasListItems := listItemPattern.MatchString(trimmed)

	// Complexity bonus: each structural feature adds ~50 runes equivalent
	complexityBonus := 0
	if hasCodeBlock {
		complexityBonus += 200
	}
	if hasURL {
		complexityBonus += 50
	}
	if hasListItems {
		complexityBonus += 50
	}
	if sentenceCount > 3 {
		complexityBonus += (sentenceCount - 3) * 30
	}

	effectiveLength := runeCount + complexityBonus

	switch {
	case effectiveLength < 20:
		return InputScaleMicro
	case effectiveLength < 100:
		return InputScaleSmall
	case effectiveLength < 500:
		return InputScaleMedium
	case effectiveLength < 2000:
		return InputScaleLarge
	default:
		return InputScaleXLarge
	}
}

var (
	// urlPattern matches http/https URLs
	urlPattern = regexp.MustCompile(`https?://\S+`)
	// listItemPattern matches markdown list items or numbered lists
	listItemPattern = regexp.MustCompile(`(?m)^[\s]*[-*+]\s|^[\s]*\d+\.\s`)
	// greetingPatterns matches common greeting and simple inquiry patterns
	greetingPatterns = regexp.MustCompile(`(?i)^(` +
		// Chinese greetings
		`你好|您好|嗨|哈喽|早上好|下午好|晚上好|` +
		// English greetings
		`hi|hello|hey|good\s*(morning|afternoon|evening)|howdy|greetings|` +
		// Simple identity/capability queries
		`你是谁|你是什么|你能做什么|你会什么|你的功能|你的能力|` +
		`who\s+are\s+you|what\s+can\s+you\s+do|what\s+are\s+you|` +
		// Status checks
		`ping|status|test|在吗|在不在|` +
		// Simple thanks
		`谢谢|感谢|thanks|thank\s+you|thx` +
		`)[\s?!。！？,.]*$`)
	// webSearchPatterns matches explicit internet search intent keywords
	webSearchPatterns = regexp.MustCompile(`(?i)(` +
		`搜索互联网|互联网搜索|网页搜索|网上搜索|在线搜索|上网搜索|上网查|` +
		`搜索引擎|百度搜索|谷歌搜索|搜一下|搜一搜|帮我搜|帮我查一下|` +
		`search\s*(the\s*)?(internet|web|online)|web\s*search|internet\s*search|google\s*search` +
		`)`)
)

// countSentences counts the approximate number of sentences in the input.
func countSentences(input string) int {
	// Split by common sentence terminators
	count := 0
	for _, r := range input {
		switch r {
		case '.', '。', '!', '！', '?', '？', '\n':
			count++
		}
	}
	// At least 1 sentence if there's content
	if count == 0 && len(input) > 0 {
		count = 1
	}
	return count
}

// FastMatchResult holds the result of fast intent matching for Micro/Small inputs.
type FastMatchResult struct {
	// IsSimpleQuery indicates the input is a greeting, status check,
	// or other trivial query that can be answered directly.
	IsSimpleQuery bool

	// WebSearchKeywordDetected indicates explicit web search keywords were found in input
	WebSearchKeywordDetected bool

	// MatchedTools contains tools found via BM25 search
	MatchedTools []*schema.AIYakTool

	// MatchedForges contains forges found via keyword matching
	MatchedForges []*schema.AIForge

	// MatchedLoops contains loop metadata matched by description
	MatchedLoops []*reactloops.LoopMetadata

	// MatchedSkills contains skills explicitly matched by name mention.
	MatchedSkills []*aiskillloader.SkillMeta

	// MatchedCapabilityMentions contains exact-name capability mentions found in user input.
	MatchedCapabilityMentions *reactloops.CapabilityNameMatchResult

	// ContextSummary is a pre-formatted string summarizing matched capabilities
	ContextSummary string
}

// HasMatches returns true if any tools, forges, or loops were matched.
func (r *FastMatchResult) HasMatches() bool {
	return len(r.MatchedTools) > 0 || len(r.MatchedForges) > 0 || len(r.MatchedLoops) > 0 || len(r.MatchedSkills) > 0 || (r.MatchedCapabilityMentions != nil && r.MatchedCapabilityMentions.HasMatches())
}

// NeedsDeepAnalysis returns true when fast matching is insufficient and
// the input should be escalated to deep intent recognition.
//
// This happens when:
//   - The input is NOT a simple query (not a greeting/status check)
//   - Fast BM25 + keyword search found NO matches
//
// Typical example: "我想做渗透测试" — short input, but represents a
// composite/multi-step task that no single tool can fulfill.
// The system cannot understand the intent with fast matching alone,
// so it must escalate to the AI-powered deep intent loop for
// task decomposition and capability discovery.
func (r *FastMatchResult) NeedsDeepAnalysis() bool {
	if r.IsSimpleQuery {
		return false
	}
	return !r.HasMatches()
}

// FastIntentMatch performs fast intent matching for Micro/Small inputs.
// It uses rule-based greeting detection and BM25 trigram search to quickly
// identify the user's intent without calling any AI model.
func FastIntentMatch(r aicommon.AIInvokeRuntime, input string) *FastMatchResult {
	trimmed := strings.TrimSpace(input)
	result := &FastMatchResult{}

	// Step 1: Greeting / simple query detection via regex
	if greetingPatterns.MatchString(trimmed) {
		result.IsSimpleQuery = true
		result.ContextSummary = "simple_query: greeting or trivial inquiry detected"
		log.Infof("fast intent match: simple query detected for input: %s", trimmed)
		return result
	}

	// Step 2a: Web search keyword shortcut — bypass BM25 ranking issues
	webSearchKeywordDetected := webSearchPatterns.MatchString(trimmed)
	if webSearchKeywordDetected {
		log.Infof("fast intent match: web search keyword detected in input: %s", trimmed)
	}

	// Step 2b: BM25 Trigram + keyword dual-channel search for tools, forges, and loops
	db := consts.GetGormProfileDatabase()
	if db != nil {
		tools, err := yakit.SearchAIYakToolBM25(db, &yakit.AIYakToolFilter{
			Keywords: []string{trimmed},
		}, 5, 0)
		if err != nil {
			log.Warnf("fast intent match: BM25 tool search failed: %v", err)
		} else if len(tools) > 0 {
			result.MatchedTools = tools
			log.Infof("fast intent match: found %d tools via BM25 for: %s", len(tools), trimmed)
		}

		// When web search keywords detected, ensure web_search tool is in results
		if webSearchKeywordDetected {
			hasWebSearch := false
			for _, t := range result.MatchedTools {
				if t.Name == "web_search" {
					hasWebSearch = true
					break
				}
			}
			if !hasWebSearch {
				webSearchTools, wsErr := yakit.SearchAIYakToolBM25(db, &yakit.AIYakToolFilter{
					Keywords: []string{"web_search"},
				}, 1, 0)
				if wsErr == nil && len(webSearchTools) > 0 {
					result.MatchedTools = append(result.MatchedTools, webSearchTools[0])
					log.Infof("fast intent match: forcibly added web_search tool via keyword shortcut")
				}
			}
		}

		forges, err := yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{
			ForgeTypes: schema.RunnableForgeTypes(),
			Keywords:   []string{trimmed},
		}, 5, 0)
		if err != nil {
			log.Warnf("fast intent match: BM25 forge search failed: %v", err)
		} else if len(forges) > 0 {
			result.MatchedForges = forges
			log.Infof("fast intent match: found %d forges via BM25 for: %s", len(forges), trimmed)
		}
	}

	// Search registered loop metadata (in-memory, not DB-backed)
	matchedLoops := matchLoopMetadata(trimmed)
	if len(matchedLoops) > 0 {
		result.MatchedLoops = matchedLoops
		log.Infof("fast intent match: found %d matching loops for: %s", len(matchedLoops), trimmed)
	}

	// When web search keywords detected, ensure internet_research loop is in results
	if webSearchKeywordDetected {
		hasInternetResearch := false
		for _, m := range result.MatchedLoops {
			if m.Name == "internet_research" {
				hasInternetResearch = true
				break
			}
		}
		if !hasInternetResearch {
			for _, meta := range reactloops.GetAllLoopMetadata() {
				if meta.Name == "internet_research" {
					result.MatchedLoops = append(result.MatchedLoops, meta)
					log.Infof("fast intent match: forcibly added internet_research loop via keyword shortcut")
					break
				}
			}
		}
	}

	result.WebSearchKeywordDetected = webSearchKeywordDetected

	// Build context summary
	if result.HasMatches() {
		result.ContextSummary = buildFastMatchSummary(result)
	}

	return result
}

// containsAnyToken checks if the searchFields contain any word-level token from the input.
// Requires at least half of meaningful tokens (len >= 2) to match.
// For single-token input, falls back to direct substring match.
func containsAnyToken(searchFields, input string) bool {
	tokens := strings.Fields(input)
	if len(tokens) <= 1 {
		return false
	}
	meaningfulTokens := 0
	matchCount := 0
	for _, token := range tokens {
		if len(token) < 2 {
			continue
		}
		meaningfulTokens++
		if strings.Contains(searchFields, strings.ToLower(token)) {
			matchCount++
		}
	}
	if meaningfulTokens == 0 {
		return false
	}
	// Require at least half of meaningful tokens to match
	return matchCount > 0 && matchCount >= (meaningfulTokens+1)/2
}

// matchLoopMetadata checks all registered loop metadata for keyword matches.
// Since LoopMetadata is in-memory (not DB-backed), this uses token-level matching.
// It checks both full-string containment and token-level overlap.
func matchLoopMetadata(input string) []*reactloops.LoopMetadata {
	allMeta := reactloops.GetAllLoopMetadata()
	inputLower := strings.ToLower(input)
	var matched []*reactloops.LoopMetadata

	for _, meta := range allMeta {
		if meta.IsHidden {
			continue
		}
		searchText := strings.ToLower(meta.Name + " " + meta.Description + " " + meta.UsagePrompt)
		if strings.Contains(searchText, inputLower) || containsAnyToken(searchText, inputLower) {
			matched = append(matched, meta)
		}
	}
	return matched
}

// buildFastMatchSummary creates a formatted summary of fast match results
// for injection into the loop context.
func buildFastMatchSummary(result *FastMatchResult) string {
	var sb strings.Builder
	sb.WriteString("## Intent Quick Match Results\n\n")

	if len(result.MatchedTools) > 0 {
		sb.WriteString("### Matched Tools\n")
		for _, tool := range result.MatchedTools {
			name := tool.Name
			if tool.VerboseName != "" {
				name = tool.VerboseName + " (" + tool.Name + ")"
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", name, truncateString(tool.Description, 120)))
		}
		sb.WriteString("\n")
	}

	if len(result.MatchedForges) > 0 {
		sb.WriteString("### Matched AI Forges (Blueprints)\n")
		for _, forge := range result.MatchedForges {
			name := forge.ForgeName
			if forge.ForgeVerboseName != "" {
				name = forge.ForgeVerboseName + " (" + forge.ForgeName + ")"
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", name, truncateString(forge.Description, 120)))
		}
		sb.WriteString("\n")
	}

	if len(result.MatchedLoops) > 0 {
		sb.WriteString("### Matched Focus Modes\n")
		for _, loop := range result.MatchedLoops {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", loop.Name, truncateString(loop.Description, 120)))
		}
		sb.WriteString("\n")
	}

	if len(result.MatchedSkills) > 0 {
		sb.WriteString("### Matched Skills\n")
		for _, skill := range result.MatchedSkills {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name, truncateString(skill.Description, 120)))
		}
		sb.WriteString("\n")
	}

	if result.MatchedCapabilityMentions != nil && result.MatchedCapabilityMentions.HasMatches() {
		if section := result.MatchedCapabilityMentions.RenderYakScriptMarkdown("### Matched Yakit Plugins"); section != "" {
			sb.WriteString(section)
		}
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// truncateString truncates a string to the specified max rune length.
func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen]) + "..."
}

// applyFastMatchResult injects fast match results into the loop context
// and populates ExtraCapabilities with matched tools, forges, and focus modes.
func applyFastMatchResult(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop, result *FastMatchResult) {
	if result == nil {
		return
	}

	if result.IsSimpleQuery {
		loop.Set("intent_hint", "simple_query")
		loop.Set("intent_scale", "micro_or_small")
		r.AddToTimeline("intent_classification", "Input classified as simple query (greeting/status). Prefer directly_answer action.")
		log.Infof("intent classification: simple query, hint set to directly_answer")
		return
	}

	if result.HasMatches() {
		loop.Set("intent_hint", "capabilities_matched")
		loop.Set("intent_scale", "micro_or_small")
		loop.Set("intent_matched_capabilities", result.ContextSummary)
		r.AddToTimeline("intent_context", result.ContextSummary)
		log.Infof("intent classification: capabilities matched via fast BM25 search")

		// Populate ExtraCapabilities from fast match results
		populateExtraCapabilitiesFromFastMatch(r, loop, result)

		if result.WebSearchKeywordDetected {
			r.AddToTimeline("web_search_keyword_detected",
				"[CRITICAL] User's input explicitly contains internet search keywords. "+
					"You MUST use web_search tool or switch to internet_research focus mode to satisfy this request. "+
					"Do NOT use do_http_request with placeholder URLs. "+
					"Do NOT rely solely on knowledge_enhance_answer. "+
					"The user is requesting real-time internet search results.")
		}

		for _, schTool := range result.MatchedTools {
			if schTool.Name == "web_search" {
				r.AddToTimeline("web_search_recommended",
					"web_search tool was identified as relevant for this query. "+
						"If knowledge_enhance_answer or knowledge base search cannot provide sufficient results, "+
						"you MUST call web_search to find the answer from the internet. "+
						"Do NOT repeatedly retry knowledge_enhance_answer if it already failed.")
				break
			}
		}
	}
}

func applyCapabilityMatchesToFastMatchResult(result *FastMatchResult, matches *reactloops.CapabilityNameMatchResult) {
	if result == nil || matches == nil || !matches.HasMatches() {
		return
	}

	result.MatchedCapabilityMentions = matches

	toolSeen := make(map[string]bool)
	for _, tool := range result.MatchedTools {
		if tool != nil {
			toolSeen[tool.Name] = true
		}
	}
	for _, tool := range matches.MatchedAITools {
		if tool == nil || toolSeen[tool.Name] {
			continue
		}
		toolSeen[tool.Name] = true
		result.MatchedTools = append(result.MatchedTools, tool)
	}

	forgeSeen := make(map[string]bool)
	for _, forge := range result.MatchedForges {
		if forge != nil {
			forgeSeen[forge.ForgeName] = true
		}
	}
	for _, forge := range matches.MatchedForges {
		if forge == nil || forgeSeen[forge.ForgeName] {
			continue
		}
		forgeSeen[forge.ForgeName] = true
		result.MatchedForges = append(result.MatchedForges, forge)
	}

	skillSeen := make(map[string]bool)
	for _, skill := range result.MatchedSkills {
		if skill != nil {
			skillSeen[skill.Name] = true
		}
	}
	for _, skill := range matches.MatchedSkills {
		if skill == nil || skillSeen[skill.Name] {
			continue
		}
		skillSeen[skill.Name] = true
		result.MatchedSkills = append(result.MatchedSkills, skill)
	}

	result.ContextSummary = buildFastMatchSummary(result)
}

// populateExtraCapabilitiesFromFastMatch adds fast match results to the loop's ExtraCapabilitiesManager.
// Fast match already has resolved objects (schema.AIYakTool, schema.AIForge, LoopMetadata),
// so no name-to-object resolution is needed.
func populateExtraCapabilitiesFromFastMatch(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop, result *FastMatchResult) {
	ecm := loop.GetExtraCapabilities()
	if ecm == nil {
		return
	}

	addedToolNames := make(map[string]bool)

	// Convert schema.AIYakTool to aitool.Tool and add
	if len(result.MatchedTools) > 0 {
		toolMgr := r.GetConfig().GetAiToolManager()
		if toolMgr != nil {
			for _, schTool := range result.MatchedTools {
				addedToolNames[schTool.Name] = true
				tool, err := toolMgr.GetToolByName(schTool.Name)
				if err != nil {
					log.Debugf("extra capabilities (fast): skip tool %q: %v", schTool.Name, err)
					continue
				}
				ecm.AddTools(tool)
			}
		}
	}

	if result.MatchedCapabilityMentions != nil && result.MatchedCapabilityMentions.HasMatches() {
		toolMgr := r.GetConfig().GetAiToolManager()
		if toolMgr != nil {
			for _, name := range result.MatchedCapabilityMentions.ToolNames() {
				if addedToolNames[name] {
					continue
				}
				tool, err := toolMgr.GetToolByName(name)
				if err != nil {
					log.Debugf("extra capabilities (fast): skip yak script %q: %v", name, err)
					continue
				}
				addedToolNames[name] = true
				ecm.AddTools(tool)
			}
		}
	}

	if len(result.MatchedSkills) > 0 {
		for _, skill := range result.MatchedSkills {
			if skill == nil {
				continue
			}
			ecm.AddSkills(reactloops.ExtraSkillInfo{
				Name:        skill.Name,
				Description: skill.Description,
			})
		}
	}

	// Add matched forges
	if len(result.MatchedForges) > 0 {
		for _, forge := range result.MatchedForges {
			ecm.AddForges(reactloops.ExtraForgeInfo{
				Name:        forge.ForgeName,
				VerboseName: forge.ForgeVerboseName,
				Description: forge.Description,
			})
		}
	}

	// Add matched focus modes (loops)
	if len(result.MatchedLoops) > 0 {
		for _, meta := range result.MatchedLoops {
			ecm.AddFocusModes(reactloops.ExtraFocusModeInfo{
				Name:        meta.Name,
				Description: meta.Description,
			})
		}
	}

	if ecm.HasCapabilities() {
		log.Infof("extra capabilities populated from fast match: %d tools, %d forges, %d skills, %d focus modes",
			ecm.ToolCount(), len(ecm.ListForges()), len(ecm.ListSkills()), len(ecm.ListFocusModes()))
	}
}

// executeDeepIntentRecognition delegates to the shared implementation in reactloops.
func executeDeepIntentRecognition(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) *reactloops.DeepIntentResult {
	return reactloops.ExecuteDeepIntentRecognition(r, loop, task)
}

// applyDeepIntentResult delegates to the shared implementation in reactloops.
func applyDeepIntentResult(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop, result *reactloops.DeepIntentResult) {
	reactloops.ApplyDeepIntentResult(r, loop, result)
}
