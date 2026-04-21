package reactloops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	PerceptionTriggerPostAction   = "post_action"
	PerceptionTriggerVerification = "verification"
	PerceptionTriggerForced       = "forced"
	PerceptionTriggerSpinDetected = "spin_detected"
	PerceptionTriggerLoopSwitch   = "loop_switch"

	perceptionDefaultMinInterval        = 30 * time.Second
	perceptionMaxInterval               = 5 * time.Minute
	perceptionDefaultIterationInterval  = 2
	perceptionMaxContextTokens          = 500
	perceptionKnowledgeMaxContextTokens = 15 * 1024

	// perceptionMaxInputTokens is the token budget for the entire perception input.
	// Fields exceeding their share will be shrunk via ShrinkTextBlockByTokens.
	perceptionMaxInputTokens = 30000
)

var perceptionCapabilitySearcher = SearchCapabilities
var perceptionKnowledgeBaseNameLister = func() ([]string, error) {
	return yakit.GetKnowledgeBaseNameList(consts.GetGormProfileDatabase())
}

// PerceptionState holds the structured output of a single perception evaluation.
// It captures what the user is currently doing in concise, searchable form.
type PerceptionState struct {
	Topics          []string  `json:"topics"`
	Keywords        []string  `json:"keywords"`
	OneLinerSummary string    `json:"summary"`
	ConfidenceLevel float64   `json:"confidence"`
	Changed         bool      `json:"changed"`
	Epoch           int       `json:"epoch"`
	LastTrigger     string    `json:"last_trigger"`
	LastUpdateAt    time.Time `json:"last_update_at"`
	PrevTopicsHash  string    `json:"prev_topics_hash"`
}

func hashTopics(topics []string) string {
	sorted := make([]string, len(topics))
	copy(sorted, topics)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "|")))
	return hex.EncodeToString(h[:8])
}

// ShouldUpdate determines whether a new perception result should overwrite the current state.
func (p *PerceptionState) ShouldUpdate(newState *PerceptionState) bool {
	if newState == nil {
		return false
	}
	if newState.LastTrigger == PerceptionTriggerForced ||
		newState.LastTrigger == PerceptionTriggerSpinDetected ||
		newState.LastTrigger == PerceptionTriggerLoopSwitch {
		return true
	}
	if !newState.Changed {
		return false
	}
	newHash := hashTopics(newState.Topics)
	return newHash != p.PrevTopicsHash
}

// FormatForContext renders the perception state as a concise natural-language
// block suitable for injection into the main loop prompt via ContextProvider.
func (p *PerceptionState) FormatForContext() string {
	if p == nil {
		return ""
	}
	age := time.Since(p.LastUpdateAt).Truncate(time.Second)
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("## Current Perception (epoch %d, %v ago)\n", p.Epoch, age))
	if p.OneLinerSummary != "" {
		buf.WriteString(fmt.Sprintf("Summary: %s\n", p.OneLinerSummary))
	}
	if len(p.Topics) > 0 {
		buf.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(p.Topics, ", ")))
	}
	if len(p.Keywords) > 0 {
		buf.WriteString(fmt.Sprintf("Keywords: %s\n", strings.Join(p.Keywords, ", ")))
	}
	result := buf.String()
	if aicommon.MeasureTokens(result) > perceptionMaxContextTokens {
		result = aicommon.ShrinkTextBlockByTokens(result, perceptionMaxContextTokens)
	}
	return result
}

// perceptionController manages the lifecycle and scheduling of perception evaluations.
type perceptionController struct {
	mu sync.Mutex

	current *PerceptionState
	epoch   int

	minInterval              time.Duration
	maxInterval              time.Duration
	currentInterval          time.Duration
	iterationTriggerInterval int
	consecutiveUnchanged     int

	running int32 // atomic CAS guard to prevent concurrent AI calls
}

type midtermTimelineRecallScheduler interface {
	ScheduleMidtermTimelineRecallFromPerception(summary string, topics []string, keywords []string)
}

func newPerceptionController(iterationTriggerInterval int) *perceptionController {
	if iterationTriggerInterval <= 0 {
		iterationTriggerInterval = perceptionDefaultIterationInterval
	}
	return &perceptionController{
		minInterval:              perceptionDefaultMinInterval,
		maxInterval:              perceptionMaxInterval,
		currentInterval:          perceptionDefaultMinInterval,
		iterationTriggerInterval: iterationTriggerInterval,
	}
}

func (pc *perceptionController) shouldTriggerOnIteration(iterationIndex int) bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.iterationTriggerInterval <= 0 {
		return false
	}
	return iterationIndex > 0 && iterationIndex%pc.iterationTriggerInterval == 0
}

func (pc *perceptionController) shouldSkipDueToInterval() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.current == nil {
		return false
	}
	return time.Since(pc.current.LastUpdateAt) < pc.currentInterval
}

func (pc *perceptionController) applyResult(newState *PerceptionState) (*PerceptionState, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if newState == nil {
		return pc.current, false
	}
	pc.epoch++
	newState.Epoch = pc.epoch
	newState.LastUpdateAt = time.Now()

	if pc.current == nil {
		newState.PrevTopicsHash = ""
		pc.current = newState
		pc.consecutiveUnchanged = 0
		pc.currentInterval = pc.minInterval
		return pc.current, true
	}

	if pc.current.ShouldUpdate(newState) {
		newState.PrevTopicsHash = hashTopics(newState.Topics)
		pc.current = newState
		pc.consecutiveUnchanged = 0
		pc.currentInterval = pc.minInterval
		return pc.current, true
	} else {
		pc.current.Epoch = newState.Epoch
		pc.current.LastUpdateAt = newState.LastUpdateAt
		pc.current.LastTrigger = newState.LastTrigger
		pc.consecutiveUnchanged++
		if pc.consecutiveUnchanged >= 2 {
			pc.currentInterval *= 2
			if pc.currentInterval > pc.maxInterval {
				pc.currentInterval = pc.maxInterval
			}
		}
	}
	return pc.current, false
}

func (pc *perceptionController) getCurrent() *PerceptionState {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.current
}

// buildPerceptionInput assembles input for the perception model with a generous
// token budget (~30K tokens). Individual sections are shrunk only when the total
// would exceed perceptionMaxInputTokens.
// It returns the core input string and a map of extra template variables
// (BaseFrame, Facts, Evidence, DynamicContext) for the prompt template.
func (r *ReActLoop) buildPerceptionInput(trigger string) (string, map[string]string) {
	var buf strings.Builder
	extra := make(map[string]string)

	task := r.GetCurrentTask()
	userInput := ""
	if task != nil {
		userInput = task.GetUserInput()
	}
	if userInput != "" {
		userInput = aicommon.ShrinkTextBlockByTokens(userInput, 8000)
	}
	buf.WriteString(fmt.Sprintf("User Goal: %s\n", userInput))
	buf.WriteString(fmt.Sprintf("Loop: %s, iteration %d/%d\n", r.loopName, r.currentIterationIndex, r.maxIterations))
	buf.WriteString(fmt.Sprintf("Trigger: %s\n", trigger))

	recentActions := r.GetLastNAction(5)
	if len(recentActions) > 0 {
		buf.WriteString("Recent Actions:\n")
		for _, a := range recentActions {
			buf.WriteString(fmt.Sprintf("  - [iter %d] %s (%s)\n", a.IterationIndex, a.ActionName, a.ActionType))
		}
	}

	lastSat := r.GetLastSatisfactionRecordFull()
	if lastSat != nil {
		satisfied := "unsatisfied"
		if lastSat.Satisfactory {
			satisfied = "satisfied"
		}
		reason := lastSat.Reason
		if len(reason) > 2000 {
			reason = aicommon.ShrinkTextBlockByTokens(reason, 1000)
		}
		buf.WriteString(fmt.Sprintf("Last Verification: %s - %s\n", satisfied, reason))
		if len(lastSat.NextMovements) > 0 {
			buf.WriteString("Next Movements:\n")
			for _, nm := range lastSat.NextMovements {
				content := nm.Content
				if len(content) > 1000 {
					content = aicommon.ShrinkTextBlockByTokens(content, 500)
				}
				buf.WriteString(fmt.Sprintf("  - %s\n", content))
			}
		}
	}

	diff, _ := r.GetTimelineDiff()
	if diff != "" {
		diff = aicommon.ShrinkTextBlockByTokens(diff, 4000)
		buf.WriteString(fmt.Sprintf("Recent Timeline Changes:\n%s\n", diff))
	}

	pc := r.perception
	if pc != nil {
		prev := pc.getCurrent()
		if prev != nil {
			buf.WriteString(fmt.Sprintf("\nPrevious Perception (epoch %d):\n", prev.Epoch))
			buf.WriteString(fmt.Sprintf("  Summary: %s\n", prev.OneLinerSummary))
			if len(prev.Topics) > 0 {
				buf.WriteString(fmt.Sprintf("  Topics: %s\n", strings.Join(prev.Topics, ", ")))
			}
			if len(prev.Keywords) > 0 {
				buf.WriteString(fmt.Sprintf("  Keywords: %s\n", strings.Join(prev.Keywords, ", ")))
			}
		}
	}

	baseFrame := r.GetBaseFrameContext()
	var baseFrameBuf strings.Builder
	if v, ok := baseFrame["CurrentTime"]; ok {
		baseFrameBuf.WriteString(fmt.Sprintf("Time: %v\n", v))
	}
	if v, ok := baseFrame["OSArch"]; ok {
		baseFrameBuf.WriteString(fmt.Sprintf("OS: %v\n", v))
	}
	if v, ok := baseFrame["WorkingDir"]; ok {
		baseFrameBuf.WriteString(fmt.Sprintf("WorkDir: %v\n", v))
	}
	if v, ok := baseFrame["Timeline"]; ok {
		timeline := fmt.Sprintf("%v", v)
		timeline = aicommon.ShrinkTextBlockByTokens(timeline, 3000)
		baseFrameBuf.WriteString(fmt.Sprintf("Timeline:\n%s\n", timeline))
	}
	if baseFrameStr := baseFrameBuf.String(); baseFrameStr != "" {
		extra["BaseFrame"] = baseFrameStr
	}

	if facts := strings.TrimSpace(r.Get("plan_facts")); facts != "" {
		facts = aicommon.ShrinkTextBlockByTokens(facts, 4000)
		extra["Facts"] = facts
	}

	if evidence := strings.TrimSpace(r.Get("plan_evidence")); evidence != "" {
		evidence = aicommon.ShrinkTextBlockByTokens(evidence, 4000)
		extra["Evidence"] = evidence
	}

	cfg := r.config
	if cfg != nil {
		if cpm := cfg.GetContextProviderManager(); cpm != nil {
			dynCtx := cpm.Execute(cfg, r.emitter)
			if dynCtx = strings.TrimSpace(dynCtx); dynCtx != "" {
				dynCtx = aicommon.ShrinkTextBlockByTokens(dynCtx, 4000)
				extra["DynamicContext"] = dynCtx
			}
		}
	}

	return buf.String(), extra
}

func (r *ReActLoop) buildPerceptionCapabilitySearchInput(state *PerceptionState) CapabilitySearchInput {
	if state == nil {
		return CapabilitySearchInput{}
	}

	query := strings.TrimSpace(state.OneLinerSummary)
	queries := normalizeCapabilityStrings(append(append([]string{}, state.Topics...), state.Keywords...))
	if len(queries) > 8 {
		queries = queries[:8]
	}
	if query == "" && len(queries) > 0 {
		query = strings.Join(queries, " ")
	}

	return CapabilitySearchInput{
		Query:               query,
		Queries:             queries,
		IncludeCatalogMatch: false,
		Limit:               5,
	}
}

func (r *ReActLoop) applyPerceptionCapabilitySearchResult(result *CapabilitySearchResult) {
	if r == nil || result == nil {
		return
	}

	if result.SearchResultsMarkdown != "" {
		r.Set("perception_capability_search_results", result.SearchResultsMarkdown)
	}
	if result.ContextEnrichment != "" {
		r.Set("perception_capability_context_enrichment", result.ContextEnrichment)
	}
	if len(result.MatchedToolNames) > 0 {
		r.Set("perception_matched_tool_names", strings.Join(result.MatchedToolNames, ","))
	}
	if len(result.MatchedForgeNames) > 0 {
		r.Set("perception_matched_forge_names", strings.Join(result.MatchedForgeNames, ","))
	}
	if len(result.MatchedSkillNames) > 0 {
		r.Set("perception_matched_skill_names", strings.Join(result.MatchedSkillNames, ","))
	}
	if len(result.MatchedFocusModeNames) > 0 {
		r.Set("perception_matched_focus_mode_names", strings.Join(result.MatchedFocusModeNames, ","))
	}
	if len(result.RecommendedCapabilities) > 0 {
		r.Set("perception_recommended_capabilities", strings.Join(result.RecommendedCapabilities, ","))
		PreloadSingleRecommendedTool(r, result.RecommendedCapabilities)
	}

	PopulateExtraCapabilitiesFromCapabilitySearchResult(r.GetInvoker(), r, result)
}

func (r *ReActLoop) clearPerceptionKnowledgeSearchResult() {
	if r == nil {
		return
	}
	for _, key := range []string{
		"perception_selected_knowledge_bases",
		"perception_knowledge_query",
		"perception_knowledge_context",
	} {
		r.Delete(key)
	}
}

func (r *ReActLoop) allowPerceptionKnowledgeRefresh() bool {
	if r == nil {
		return false
	}
	if r.allowRAG == nil {
		return false
	}
	return r.allowRAG()
}

func (r *ReActLoop) buildPerceptionKnowledgeSearchQuery(state *PerceptionState) string {
	if state == nil {
		return ""
	}

	var parts []string
	if summary := strings.TrimSpace(state.OneLinerSummary); summary != "" {
		parts = append(parts, summary)
	}
	if topics := normalizeCapabilityStrings(state.Topics); len(topics) > 0 {
		parts = append(parts, "Topics: "+strings.Join(topics, ", "))
	}
	if keywords := normalizeCapabilityStrings(state.Keywords); len(keywords) > 0 {
		parts = append(parts, "Keywords: "+strings.Join(keywords, ", "))
	}

	query := strings.TrimSpace(strings.Join(parts, "\n"))
	if query == "" {
		return ""
	}
	return aicommon.ShrinkTextBlockByTokens(query, 2048)
}

func (r *ReActLoop) buildPerceptionKnowledgeKeywordQuery(state *PerceptionState) string {
	if state == nil {
		return ""
	}

	values := normalizeCapabilityStrings(append(append([]string{}, state.Keywords...), state.Topics...))
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, " ")
}

func splitPerceptionKnowledgeBaseNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return normalizeCapabilityStrings(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	}))
}

func (r *ReActLoop) resolvePerceptionKnowledgeBases(
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	searchQuery string,
) []string {
	if r == nil || utils.IsNil(invoker) {
		return nil
	}

	allKBNames, err := perceptionKnowledgeBaseNameLister()
	if err != nil {
		log.Warnf("perception knowledge: failed to load all knowledge base names: %v", err)
	} else {
		allKBNames = normalizeCapabilityStrings(allKBNames)
		if len(allKBNames) > 0 {
			return allKBNames
		}
	}

	if kbNames := splitPerceptionKnowledgeBaseNames(r.Get("knowledge_bases")); len(kbNames) > 0 {
		return kbNames
	}

	task := r.GetCurrentTask()
	var knowledgeBases []string
	includeAllKnowledgeBases := false
	autoSelectKnowledgeBases := false
	if task != nil {
		for _, data := range task.GetAttachedDatas() {
			if data == nil || data.Type != aicommon.CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE {
				continue
			}
			if data.Key == aicommon.CONTEXT_PROVIDER_KEY_SYSTEM_FLAG {
				switch {
				case data.Value == aicommon.CONTEXT_PROVIDER_VALUE_ALL_KNOWLEDGE_BASE:
					includeAllKnowledgeBases = true
					autoSelectKnowledgeBases = false
					continue
				case strings.HasPrefix(data.Value, aicommon.CONTEXT_PROVIDER_VALUE_AUTO_SELECT_KNOWLEDGE_BASE):
					autoSelectKnowledgeBases = true
					includeAllKnowledgeBases = false
					continue
				}
			}
			knowledgeBases = append(knowledgeBases, data.Value)
		}
	}

	if includeAllKnowledgeBases {
		knowledgeBases = append(knowledgeBases, allKBNames...)
	}
	if autoSelectKnowledgeBases {
		knowledgeBases = nil
	}

	knowledgeBases = normalizeCapabilityStrings(knowledgeBases)
	if len(knowledgeBases) > 0 {
		return knowledgeBases
	}

	selectResult, err := invoker.SelectKnowledgeBase(ctx, searchQuery)
	if err != nil {
		log.Warnf("perception knowledge: select knowledge bases failed: %v", err)
		return nil
	}
	if selectResult == nil {
		return nil
	}
	return normalizeCapabilityStrings(selectResult.KnowledgeBases)
}

func formatPerceptionKnowledgeContext(query string, knowledgeBases []string, content string) string {
	query = strings.TrimSpace(query)
	content = strings.TrimSpace(content)
	knowledgeBases = normalizeCapabilityStrings(knowledgeBases)

	if query == "" || content == "" {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("## Perception Knowledge\n")
	buf.WriteString("This knowledge was refreshed after a perception update.\n")
	if len(knowledgeBases) > 0 {
		buf.WriteString("Knowledge Bases: ")
		buf.WriteString(strings.Join(knowledgeBases, ", "))
		buf.WriteString("\n")
	}
	buf.WriteString("Query: ")
	buf.WriteString(query)
	buf.WriteString("\n\n")
	buf.WriteString(content)

	result := buf.String()
	if aicommon.MeasureTokens(result) > perceptionKnowledgeMaxContextTokens {
		result = aicommon.ShrinkTextBlockByTokens(result, perceptionKnowledgeMaxContextTokens)
	}
	return strings.TrimSpace(result)
}

func (r *ReActLoop) applyPerceptionKnowledgeSearchResult(query string, knowledgeBases []string, content string) {
	if r == nil {
		return
	}

	r.clearPerceptionKnowledgeSearchResult()
	query = strings.TrimSpace(query)
	knowledgeBases = normalizeCapabilityStrings(knowledgeBases)
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	if query != "" {
		r.Set("perception_knowledge_query", query)
	}
	if len(knowledgeBases) > 0 {
		r.Set("perception_selected_knowledge_bases", strings.Join(knowledgeBases, ","))
	}
	r.Set("perception_knowledge_context", content)
}

func (r *ReActLoop) refreshKnowledgeFromPerception(state *PerceptionState) {
	if r == nil || state == nil {
		return
	}

	r.clearPerceptionKnowledgeSearchResult()
	if !r.allowPerceptionKnowledgeRefresh() {
		return
	}

	invoker := r.GetInvoker()
	if utils.IsNil(invoker) {
		return
	}

	task := r.GetCurrentTask()
	if task == nil {
		return
	}

	ctx := r.config.GetContext()
	if taskCtx := task.GetContext(); !utils.IsNil(taskCtx) {
		ctx = taskCtx
	}

	searchQuery := r.buildPerceptionKnowledgeSearchQuery(state)
	if searchQuery == "" {
		return
	}

	knowledgeBases := r.resolvePerceptionKnowledgeBases(ctx, invoker, searchQuery)
	if len(knowledgeBases) == 0 {
		log.Debugf("perception knowledge: no knowledge bases available for query: %s", utils.ShrinkString(searchQuery, 120))
		return
	}

	usedQuery := searchQuery
	enhancePlans := []string{"hypothetical_answer", "generalize_query", "split_query"}
	enhanceData, err := invoker.EnhanceKnowledgeGetterEx(ctx, usedQuery, enhancePlans, knowledgeBases...)
	if err != nil {
		log.Warnf("perception knowledge search failed: %v", err)
		return
	}

	if strings.TrimSpace(enhanceData) == "" {
		if keywordQuery := r.buildPerceptionKnowledgeKeywordQuery(state); keywordQuery != "" && keywordQuery != usedQuery {
			usedQuery = keywordQuery
			enhanceData, err = invoker.EnhanceKnowledgeGetterEx(ctx, usedQuery, []string{"exact_keyword_search"}, knowledgeBases...)
			if err != nil {
				log.Warnf("perception knowledge keyword fallback failed: %v", err)
				return
			}
		}
	}

	enhanceData = strings.TrimSpace(enhanceData)
	if enhanceData == "" {
		return
	}

	compressed, err := invoker.CompressLongTextWithDestination(ctx, enhanceData, usedQuery, perceptionKnowledgeMaxContextTokens)
	if err != nil {
		log.Warnf("perception knowledge compression failed: %v", err)
		compressed = enhanceData
	}
	compressed = strings.TrimSpace(compressed)
	if compressed == "" {
		compressed = enhanceData
	}

	contextBlock := formatPerceptionKnowledgeContext(usedQuery, knowledgeBases, compressed)
	if contextBlock == "" {
		return
	}

	r.applyPerceptionKnowledgeSearchResult(usedQuery, knowledgeBases, contextBlock)
}

func (r *ReActLoop) refreshCapabilitiesFromPerception(state *PerceptionState) {
	if r == nil || state == nil {
		return
	}

	invoker := r.GetInvoker()
	if utils.IsNil(invoker) {
		return
	}

	input := r.buildPerceptionCapabilitySearchInput(state)
	if strings.TrimSpace(input.Query) == "" && len(input.Queries) == 0 {
		writePerceptionDebugMarkdown(r, state, input, nil, nil)
		return
	}

	searchResult, err := perceptionCapabilitySearcher(invoker, r, input)
	defer writePerceptionDebugMarkdown(r, state, input, searchResult, err)
	if err != nil {
		log.Warnf("perception capability search failed (epoch=%d, trigger=%s): %v", state.Epoch, state.LastTrigger, err)
		return
	}
	if searchResult == nil {
		return
	}

	r.applyPerceptionCapabilitySearchResult(searchResult)

	if len(searchResult.MatchedToolNames) > 0 || len(searchResult.MatchedForgeNames) > 0 || len(searchResult.MatchedSkillNames) > 0 || len(searchResult.MatchedFocusModeNames) > 0 {
		invoker.AddToTimeline("perception_capabilities",
			fmt.Sprintf("Perception capability search (epoch=%d): tools=%d, forges=%d, skills=%d, focus_modes=%d",
				state.Epoch, len(searchResult.MatchedToolNames), len(searchResult.MatchedForgeNames), len(searchResult.MatchedSkillNames), len(searchResult.MatchedFocusModeNames)))
	}
}

// perceptionOutputSchema defines the JSON Schema for perception AI output
// via LiteForge's aitool.ToolOption mechanism.
var perceptionOutputSchema = []aitool.ToolOption{
	aitool.WithStringParam("summary",
		aitool.WithParam_Description("用一句话概括用户当前在做什么 (max 80 chars, match user language) / One sentence summarizing what the user is doing"),
		aitool.WithParam_Required(true),
	),
	aitool.WithStringArrayParam("topics",
		aitool.WithParam_Description("2-5 个当前问题域的主题短语 / 2-5 noun phrases describing the current problem domain"),
		aitool.WithParam_Required(true),
	),
	aitool.WithStringArrayParam("keywords",
		aitool.WithParam_Description("3-8 个可检索的精确关键词 / 3-8 searchable keywords for tools or knowledge retrieval"),
		aitool.WithParam_Required(true),
	),
	aitool.WithBoolParam("changed",
		aitool.WithParam_Description("自上次感知以来情况是否发生实质性变化 / Whether the situation meaningfully changed since previous perception"),
		aitool.WithParam_Required(true),
	),
	aitool.WithNumberParam("confidence",
		aitool.WithParam_Description("对本次感知结果的置信度 / Confidence in this assessment, 0.0-1.0"),
		aitool.WithParam_Required(true),
		aitool.WithParam_Min(0),
		aitool.WithParam_Max(1),
	),
}

// TriggerPerception runs a lightweight AI evaluation to sense what the user is
// currently doing. If force is true, it bypasses interval and delta checks.
func (r *ReActLoop) TriggerPerception(reason string, force bool) *PerceptionState {
	if r.perception == nil {
		return nil
	}

	if !atomic.CompareAndSwapInt32(&r.perception.running, 0, 1) {
		log.Debugf("perception skipped: another perception call is already running (trigger=%s)", reason)
		return r.perception.getCurrent()
	}
	defer atomic.StoreInt32(&r.perception.running, 0)

	if !force && r.perception.shouldSkipDueToInterval() {
		log.Debugf("perception skipped: interval not reached (trigger=%s)", reason)
		return r.perception.getCurrent()
	}

	input, extra := r.buildPerceptionInput(reason)
	prompt, err := buildPerceptionPrompt(input, extra)
	if err != nil {
		log.Warnf("perception prompt build failed: %v", err)
		return r.perception.getCurrent()
	}

	invoker := r.GetInvoker()
	if utils.IsNil(invoker) {
		log.Warn("perception: invoker is nil")
		return r.perception.getCurrent()
	}

	ctx := r.config.GetContext()
	if task := r.GetCurrentTask(); task != nil {
		ctx = task.GetContext()
	}

	action, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx, "perception", prompt, perceptionOutputSchema,
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("perception", "summary"),
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("perception", "topics"),
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("perception", "keywords"),
	)
	if err != nil {
		log.Warnf("perception liteforge call failed (trigger=%s): %v", reason, err)
		return r.perception.getCurrent()
	}
	if utils.IsNil(action) {
		log.Warnf("perception: action is nil (trigger=%s)", reason)
		return r.perception.getCurrent()
	}

	params := action.GetParams()
	parsed := &PerceptionState{
		OneLinerSummary: params.GetString("summary"),
		Topics:          params.GetStringSlice("topics"),
		Keywords:        params.GetStringSlice("keywords"),
		Changed:         params.GetBool("changed"),
		ConfidenceLevel: params.GetFloat("confidence"),
	}

	parsed.LastTrigger = reason
	currentState, updated := r.perception.applyResult(parsed)
	if updated {
		r.refreshCapabilitiesFromPerception(currentState)
		r.refreshKnowledgeFromPerception(currentState)
	}

	if scheduler, ok := invoker.(midtermTimelineRecallScheduler); ok {
		summaryForMidterm := strings.TrimSpace(parsed.OneLinerSummary)
		if summaryForMidterm == "" {
			if current := r.perception.getCurrent(); current != nil {
				summaryForMidterm = strings.TrimSpace(current.OneLinerSummary)
			}
		}
		scheduler.ScheduleMidtermTimelineRecallFromPerception(summaryForMidterm, parsed.Topics, parsed.Keywords)
	}

	invoker.AddToTimeline("perception",
		fmt.Sprintf("Perception (epoch %d, trigger=%s): %s | topics=[%s]",
			parsed.Epoch, reason, parsed.OneLinerSummary, strings.Join(parsed.Topics, ", ")))

	log.Infof("perception updated (epoch=%d, trigger=%s, changed=%v, confidence=%.2f): %s",
		parsed.Epoch, reason, parsed.Changed, parsed.ConfidenceLevel, parsed.OneLinerSummary)

	return r.perception.getCurrent()
}

// GetCurrentTopics returns the topics from the latest perception evaluation.
func (r *ReActLoop) GetCurrentTopics() []string {
	if r.perception == nil {
		return nil
	}
	state := r.perception.getCurrent()
	if state == nil {
		return nil
	}
	return state.Topics
}

// GetCurrentKeywords returns the keywords from the latest perception evaluation.
func (r *ReActLoop) GetCurrentKeywords() []string {
	if r.perception == nil {
		return nil
	}
	state := r.perception.getCurrent()
	if state == nil {
		return nil
	}
	return state.Keywords
}

// GetPerceptionSummary returns the one-liner summary from the latest perception evaluation.
func (r *ReActLoop) GetPerceptionSummary() string {
	if r.perception == nil {
		return ""
	}
	state := r.perception.getCurrent()
	if state == nil {
		return ""
	}
	return state.OneLinerSummary
}

// IsPerceptionEnabled returns true if the perception controller is active.
func (r *ReActLoop) IsPerceptionEnabled() bool {
	return r.perception != nil
}

// GetPerceptionState returns the full current perception state.
func (r *ReActLoop) GetPerceptionState() *PerceptionState {
	if r.perception == nil {
		return nil
	}
	return r.perception.getCurrent()
}

// RegisterPerceptionContextProvider registers a traced ContextProvider that injects
// the current perception state into the main loop's prompt on each iteration.
func (r *ReActLoop) RegisterPerceptionContextProvider() {
	if r.perception == nil {
		return
	}
	mgr := r.config.GetContextProviderManager()
	if mgr == nil {
		return
	}
	mgr.RegisterTracedContent("perception_awareness", func(
		config aicommon.AICallerConfigIf,
		emitter *aicommon.Emitter,
		key string,
	) (string, error) {
		state := r.GetPerceptionState()
		if state == nil {
			return "", nil
		}
		return state.FormatForContext(), nil
	})
	mgr.RegisterTracedContent("perception_knowledge", func(
		config aicommon.AICallerConfigIf,
		emitter *aicommon.Emitter,
		key string,
	) (string, error) {
		return strings.TrimSpace(r.Get("perception_knowledge_context")), nil
	})
	log.Infof("perception context provider registered for loop %s", r.loopName)
}

// MaybeTriggerPerceptionAfterAction conditionally triggers perception after an
// action completes. Respects iteration interval and time-based throttling.
func (r *ReActLoop) MaybeTriggerPerceptionAfterAction(iterationIndex int) {
	if r.perception == nil {
		return
	}
	if !r.perception.shouldTriggerOnIteration(iterationIndex) {
		return
	}
	go r.TriggerPerception(PerceptionTriggerPostAction, false)
}

// MaybeTriggerPerceptionAfterVerification triggers perception after a verification
// result, running in a goroutine to avoid blocking the main loop.
func (r *ReActLoop) MaybeTriggerPerceptionAfterVerification() {
	if r.perception == nil {
		return
	}
	go r.TriggerPerception(PerceptionTriggerVerification, false)
}

// TriggerPerceptionOnSpin forces a perception update when SPIN is detected,
// providing fresh context that may help the loop break out of a repeating pattern.
func (r *ReActLoop) TriggerPerceptionOnSpin() {
	if r.perception == nil {
		return
	}
	go r.TriggerPerception(PerceptionTriggerSpinDetected, true)
}
