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
	PerceptionTriggerPostAction = "post_action"
	PerceptionTriggerForced     = "forced"
	PerceptionTriggerLoopSwitch = "loop_switch"

	// perceptionDefaultMinInterval 是两次 perception AI 调用之间的最小时间间隔.
	// v4: 调整自 180s -> 5min, 配合 iter=8 进一步压低调用频率.
	// 意图感知主要服务于 capability/SKILL/knowledge 补充, 大方向不变时
	// 频繁感知反而是负担. 删除 afterVerification 入口后, perception 只由
	// post_action (iter 门) + 显式 forced 触发, 再叠加更长的 minInterval,
	// 让同阶段内紧邻的 drift 候选 (iter 间隔 <150s) 被跳过, 仅阶段切换
	// (iter 间隔 >5min) 触发.
	// 关键词: perceptionDefaultMinInterval v4 默认值, 5min 节流,
	//        afterVerification 删除, drift 跳过 pivot 保留
	perceptionDefaultMinInterval = 5 * time.Minute
	perceptionMaxInterval        = 15 * time.Minute
	// perceptionDefaultIterationInterval 控制每隔几个 iter 触发一次 post_action 感知.
	// v4: 调整自 6 -> 8, 配合 minInterval=5min 压低频率:
	//   iter=4 min=120s: 仍偏密, 删除 verification 入口后实测仍有冗余
	//   iter=6 min=180s: 首次感知推迟到 iter 6,
	//                    同阶段 drift 被 iter 门 + 时间门双跳过, 仅 pivot 放行
	// 显式 forced 路径绕过 interval 门, 不会因 iter=8 而漏掉紧急刷新.
	// 关键词: perceptionDefaultIterationInterval v4 默认值, iter=8 节流,
	//        forced 兜底, 双门控
	perceptionDefaultIterationInterval  = 8
	perceptionMaxContextTokens          = 500
	perceptionKnowledgeMaxContextTokens = 15 * 1024

	// perceptionMaxInputTokens is the token budget for the entire speed-priority
	// perception input. Keep it well below the main model context.
	// Fields exceeding their share will be shrunk via ShrinkTextBlockByTokens.
	perceptionMaxInputTokens = 20000
)

// IntentShift 取值约定 — 描述本轮 perception 与上一轮相比意图的"方向性"变化粒度.
// 这是 Changed bool 的精细化补充: Changed 决定 state 是否覆盖 (受 topics hash 影响),
// IntentShift 决定昂贵下游 (capability/knowledge/midterm timeline 召回) 是否要重新加载.
//
// 设计动机: AI 经常在同一个领域里推进时也把 Changed 标成 true, 导致下游每轮都重新刷新,
// 浪费 token 与算力, 还让前端每轮都收到新的 perception_capabilities / perception_knowledge
// 事件, 严重打扰用户. IntentShift=pivot 把"真正的方向性转向"显式分离出来作为下游门控.
//
// 关键词: PerceptionState IntentShift, 意图方向性变更, 下游昂贵刷新门控,
//
//	pivot drift none, perception 节流
const (
	// IntentShiftNone 与上一次感知相比基本没变化, 通常 Changed 应该也是 false.
	IntentShiftNone = "none"
	// IntentShiftDrift 同一问题域内的 topics 自然漂移 (例如同一个调试任务里
	// 调整了关注点), Changed 可能是 true 但意图方向没变, 不应触发昂贵下游.
	IntentShiftDrift = "drift"
	// IntentShiftPivot 用户/任务真正切换了方向 (例如从写代码切换到写测试),
	// 必须重新加载下游推荐 (capability / knowledge / midterm recall).
	IntentShiftPivot = "pivot"
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
	// IntentShift 是可选字段, 取值 IntentShiftNone / IntentShiftDrift / IntentShiftPivot.
	// AI 不返回时留空, 此时 IsIntentPivot 会回退到 Changed 的旧语义, 保证向后兼容.
	// 关键词: PerceptionState.IntentShift, 意图方向性变更可选字段
	IntentShift string `json:"intent_shift,omitempty"`
}

// IsIntentPivot 报告本轮 perception 是否应该被视作"方向性 pivot"的下游门控信号.
//
// 决策优先级:
//  1. 显式 IntentShiftPivot -> true
//  2. 显式 IntentShiftDrift / IntentShiftNone -> false
//  3. IntentShift 为空 (AI 未返回新字段) -> 回退到 Changed 的值, 保留旧版"Changed=true
//     就触发下游"的行为, 避免静默收紧导致旧模型/旧 prompt 路径下游永远不刷新.
//
// 关键词: PerceptionState.IsIntentPivot, 下游门控信号, 向后兼容回退 Changed
func (p *PerceptionState) IsIntentPivot() bool {
	if p == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(p.IntentShift)) {
	case IntentShiftPivot:
		return true
	case IntentShiftDrift, IntentShiftNone:
		return false
	default:
		return p.Changed
	}
}

// shouldRefreshDownstreamForState 集中表达"是否要触发昂贵下游刷新"的门控.
//
// 规则:
//   - state 必须实际被覆盖 (updated=true), 否则没有新内容可推, 跳过
//   - LastTrigger == PerceptionTriggerForced 时绕门 (用户/系统显式请求, 视作必须刷新)
//   - 其他场景: 必须 IsIntentPivot() == true 才触发
//
// 关键词: shouldRefreshDownstreamForState, perception 下游门控,
//
//	forced 绕门, loop_switch 受门控
func (p *PerceptionState) shouldRefreshDownstreamForState(updated bool) bool {
	if !updated || p == nil {
		return false
	}
	if p.LastTrigger == PerceptionTriggerForced {
		return true
	}
	return p.IsIntentPivot()
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
		// 退避: 连续未变化时加倍下次 interval, 让节流更快收紧.
		// v3: 阈值从 2 降到 1, 第一次 unchanged 即开始退避, 配合 minInterval=5min
		// 让同方向任务的 perception 间隔迅速爬到 maxInterval, 减少无谓调用.
		// 关键词: consecutiveUnchanged 退避阈值 1, 立即退避, 节流收紧
		if pc.consecutiveUnchanged >= 1 {
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

// buildPerceptionInput assembles input for the perception model with a bounded
// token budget (~20K tokens). Individual sections are shrunk only when the total
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
		userInput = aicommon.ShrinkTextBlockByTokens(userInput, 4000)
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
		// 注: verification 收缩为纯观测角色后不再产出 TodoDelta, 这里不再
		// 渲染 "Next Steps" 段 (TODO 推进交由主循环 todo_delta).
		// satisfied + reasoning 仍是 perception 需要感知的观测信号.
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
		if cfg, ok := r.config.(interface{ GetTimeline() *aicommon.Timeline }); ok && cfg.GetTimeline() != nil {
			timeline = cfg.GetTimeline().DumpRecentForPrompt(2048)
		} else {
			timeline = aicommon.ShrinkTextBlockByTokens(timeline, 2048)
		}
		baseFrameBuf.WriteString(fmt.Sprintf("Timeline:\n%s\n", timeline))
	}
	if baseFrameStr := baseFrameBuf.String(); baseFrameStr != "" {
		extra["BaseFrame"] = baseFrameStr
	}

	if facts := strings.TrimSpace(r.Get("plan_facts")); facts != "" {
		facts = aicommon.ShrinkTextBlockByTokens(facts, 2048)
		extra["Facts"] = facts
	}

	if evidence := strings.TrimSpace(r.Get("plan_evidence")); evidence != "" {
		evidence = aicommon.ShrinkTextBlockByTokens(evidence, 2048)
		extra["Evidence"] = evidence
	}

	cfg := r.config
	if cfg != nil {
		if cpm := cfg.GetContextProviderManager(); cpm != nil {
			dynCtx := cpm.Execute(cfg, r.emitter)
			if dynCtx = strings.TrimSpace(dynCtx); dynCtx != "" {
				dynCtx = aicommon.ShrinkTextBlockByTokens(dynCtx, 2048)
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
	_ = ctx
	_ = searchQuery
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
	return nil
}

func (r *ReActLoop) buildPerceptionKnowledgeLikeKeywords(state *PerceptionState) []string {
	if state == nil {
		return nil
	}

	keywords := normalizeCapabilityStrings(append(append([]string{}, state.Keywords...), state.Topics...))
	if len(keywords) > 0 {
		return keywords
	}

	query := r.buildPerceptionKnowledgeSearchQuery(state)
	if query == "" {
		return nil
	}
	return normalizeCapabilityStrings(strings.Fields(query))
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

	if cfg := r.config; cfg != nil {
		aicommon.SetSessionSnapshotPerceptionKnowledge(cfg, &aicommon.SessionSnapshotPerceptionKnowledge{
			Query:          query,
			KnowledgeBases: knowledgeBases,
			Content:        content,
		})
		aicommon.NotifySessionSnapshotEmit(cfg)
	}
}

func (r *ReActLoop) refreshKnowledgeFromPerception(state *PerceptionState) {
	if r == nil || state == nil {
		return
	}
	stageStart := time.Now()
	defer func() {
		setWorkspaceDebugDuration(r, perceptionDebugKnowledgeDurationKey, time.Since(stageStart))
	}()

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

	resolveStart := time.Now()
	knowledgeBases := r.resolvePerceptionKnowledgeBases(ctx, invoker, searchQuery)
	setWorkspaceDebugDuration(r, perceptionDebugKnowledgeResolveDurationKey, time.Since(resolveStart))
	if len(knowledgeBases) == 0 {
		log.Debugf("perception knowledge: no knowledge bases available for query: %s", utils.ShrinkString(searchQuery, 120))
		return
	}

	usedQuery := searchQuery
	keywordList := r.buildPerceptionKnowledgeLikeKeywords(state)
	if len(keywordList) > 0 {
		usedQuery = strings.Join(keywordList, " ")
	}
	quickSearchStart := time.Now()
	enhanceData, err := invoker.QuickKnowledgeSearch(ctx, searchQuery, keywordList, knowledgeBases...)
	setWorkspaceDebugDuration(r, perceptionDebugKnowledgeSearchDurationKey, time.Since(quickSearchStart))
	if err != nil {
		log.Warnf("perception knowledge: quick search failed: %v", err)
		return
	}
	enhanceData = strings.TrimSpace(enhanceData)
	if enhanceData == "" {
		return
	}
	compressStart := time.Now()
	compressed, compressErr := invoker.CompressLongTextWithDestination(ctx, enhanceData, usedQuery, int64(perceptionKnowledgeMaxContextTokens))
	setWorkspaceDebugDuration(r, perceptionDebugKnowledgeCompressDurationKey, time.Since(compressStart))
	if compressErr != nil {
		log.Warnf("perception knowledge: compress failed: %v", compressErr)
	} else if trimmed := strings.TrimSpace(compressed); trimmed != "" {
		enhanceData = trimmed
	}

	contextBlock := formatPerceptionKnowledgeContext(usedQuery, knowledgeBases, enhanceData)
	if contextBlock == "" {
		return
	}

	r.applyPerceptionKnowledgeSearchResult(usedQuery, knowledgeBases, contextBlock)
}

func (r *ReActLoop) refreshCapabilitiesFromPerception(state *PerceptionState) {
	if r == nil || state == nil {
		return
	}
	stageStart := time.Now()
	defer func() {
		setWorkspaceDebugDuration(r, perceptionDebugCapabilityDurationKey, time.Since(stageStart))
	}()

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

	if cfg := r.config; cfg != nil {
		aicommon.SetSessionSnapshotPerceptionCapabilityMatches(cfg, &aicommon.SessionSnapshotPerceptionCapabilityMatches{
			Query:                   input.Query,
			MatchedToolNames:        searchResult.MatchedToolNames,
			MatchedForgeNames:       searchResult.MatchedForgeNames,
			MatchedSkillNames:       searchResult.MatchedSkillNames,
			MatchedFocusModeNames:   searchResult.MatchedFocusModeNames,
			RecommendedCapabilities: searchResult.RecommendedCapabilities,
		})
		aicommon.NotifySessionSnapshotEmit(cfg)
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
	// intent_shift 是可选字段, 比 changed 更精细的方向性变更信号. 留空时
	// 系统回退到 changed 的旧语义, 不做硬性要求, 让旧 prompt 路径平滑过渡.
	// 关键词: intent_shift schema, 可选枚举字段, 下游门控信号
	aitool.WithStringParam("intent_shift",
		aitool.WithParam_Description(
			"可选: 意图方向性粒度 / Optional intent change granularity. "+
				"none=完全无变化, drift=同领域内主题漂移 (changed 可能仍是 true 但方向没变), "+
				"pivot=方向性转向 (仅 pivot 时下游会重新加载推荐). 不确定时不要填."),
		aitool.WithParam_EnumString(IntentShiftNone, IntentShiftDrift, IntentShiftPivot),
	),
}

// TriggerPerception runs a lightweight AI evaluation to sense what the user is
// currently doing. If force is true, it bypasses interval and delta checks.
func (r *ReActLoop) TriggerPerception(reason string, force bool) *PerceptionState {
	if r.perception == nil {
		return nil
	}
	totalStart := time.Now()
	defer func() {
		setWorkspaceDebugDuration(r, perceptionDebugTotalDurationKey, time.Since(totalStart))
	}()

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

	aiStart := time.Now()
	action, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx, "perception", prompt, perceptionOutputSchema,
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("perception", "summary"),
	)
	setWorkspaceDebugDuration(r, perceptionDebugAIDurationKey, time.Since(aiStart))
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
		// intent_shift 是新增的可选字段, AI 不返回时留空, IsIntentPivot 会回退到 Changed 语义.
		// 这里做 lowercase + trim 归一化, 避免大小写/前后空格让枚举判定失效.
		// 关键词: parsed.IntentShift 解析归一化, 大小写无关, intent_shift 可选解析
		IntentShift: strings.ToLower(strings.TrimSpace(params.GetString("intent_shift"))),
	}

	parsed.LastTrigger = reason
	currentState, updated := r.perception.applyResult(parsed)

	// 下游昂贵刷新统一走 IntentShift 门控.
	//
	// shouldRefreshDownstreamForState 已经合并了三条规则:
	//   1. updated 必须为 true, 否则没有新内容可以推
	//   2. forced trigger 一律绕门 (用户/系统显式请求, 必须刷新)
	//   3. 否则要求 IntentShift=pivot (向后兼容: IntentShift 空时回退到 Changed)
	//
	// 注意行为变化: ScheduleMidtermTimelineRecallFromPerception 之前是每次 TriggerPerception
	// 必调, 现在改为只在 pivot 或 forced 时调用. 这是用户明确要求的, 用于解决意图未真正
	// 变化时的中长期 timeline 召回噪音.
	//
	// 关键词: TriggerPerception 下游门控, refreshCapabilities/refreshKnowledge/ScheduleMidterm
	//        统一走 shouldRefreshDownstreamForState
	if currentState.shouldRefreshDownstreamForState(updated) {
		r.refreshCapabilitiesFromPerception(currentState)
		r.refreshKnowledgeFromPerception(currentState)
		if scheduler, ok := invoker.(midtermTimelineRecallScheduler); ok {
			summaryForMidterm := strings.TrimSpace(parsed.OneLinerSummary)
			if summaryForMidterm == "" {
				if current := r.perception.getCurrent(); current != nil {
					summaryForMidterm = strings.TrimSpace(current.OneLinerSummary)
				}
			}
			scheduler.ScheduleMidtermTimelineRecallFromPerception(summaryForMidterm, parsed.Topics, parsed.Keywords)
		}
	}

	if cfg := r.config; cfg != nil && currentState != nil {
		aicommon.NotifySessionSnapshotEmit(cfg)
	}

	invoker.AddToTimeline("perception",
		fmt.Sprintf("Perception (epoch %d, trigger=%s): %s | topics=[%s]",
			parsed.Epoch, reason, parsed.OneLinerSummary, strings.Join(parsed.Topics, ", ")))

	log.Infof("perception updated (epoch=%d, trigger=%s, changed=%v, intent_shift=%q, confidence=%.2f): %s",
		parsed.Epoch, reason, parsed.Changed, parsed.IntentShift, parsed.ConfidenceLevel, parsed.OneLinerSummary)

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

func (r *ReActLoop) invokePerceptionTrigger(reason string, force bool) {
	if r.config != nil && r.config.GetConfigBool("SyncPerceptionTrigger") {
		r.TriggerPerception(reason, force)
		return
	}
	go r.TriggerPerception(reason, force)
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
	r.invokePerceptionTrigger(PerceptionTriggerPostAction, false)
}
