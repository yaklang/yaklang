package reactloops

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

var contextOptimizationStatusIndex atomic.Uint64

var contextOptimizationStatusNames = []actionStatusName{
	{"正在优化上下文…", "Optimizing the context…"},
	{"正在整理历史记录，保留关键线索…", "Organizing the history and keeping key findings…"},
	{"正在精简上下文，保留重要信息…", "Compacting the context and keeping important information…"},
	{"正在归纳前面的进展…", "Summarizing progress so far…"},
	{"正在梳理上下文，准备继续…", "Organizing the context before continuing…"},
}

const defaultWholePromptTokenLimit int64 = 300 * 1024

// wholePromptTokenLimit is the limit for the assembled main-loop prompt.
// The configured timeline limit defaults to 300k; a too-large response lowers it.
func (r *ReActLoop) wholePromptTokenLimit() int64 {
	if r == nil || r.config == nil {
		return defaultWholePromptTokenLimit
	}
	base := r.config.GetTimelineContentSizeLimit()
	if base <= 0 {
		base = defaultWholePromptTokenLimit
	}
	config, ok := r.config.(*aicommon.Config)
	if !ok || config == nil {
		return base
	}
	return config.EffectiveTimelineTokenLimit(config.AiServerName, config.AiModelName, base)
}

func (r *ReActLoop) noteCompressionStatus() {
	if r == nil {
		return
	}
	index := (contextOptimizationStatusIndex.Add(1) - 1) % uint64(len(contextOptimizationStatusNames))
	label := contextOptimizationStatusNames[index]
	r.UserStatus(label.zh, label.en, aicommon.WithStatusCode("context.optimizing"))
}

func (r *ReActLoop) parseContextCompressionSelection(action *aicommon.Action, candidates []aicommon.TimelineCompressionCandidate, maxSummaryTokens int) (*aicommon.TimelineCompressionSelection, error) {
	if action == nil {
		return nil, fmt.Errorf("context compression requires an action")
	}
	rawSummary, _ := action.LookupCanonicalParam("summary")
	summary, ok := rawSummary.(string)
	if !ok {
		return nil, fmt.Errorf("summary 必须是字符串")
	}
	summary = strings.TrimSpace(summary)
	rawRange, _ := action.LookupCanonicalParam("retain_timeline")
	retainedRange, ok := rawRange.(string)
	if !ok {
		return nil, fmt.Errorf("retain_timeline 必须是 ID/范围字符串，例如 12,18-23；全部概括时返回空字符串")
	}
	retained, err := parseContextCompressionRetainedIDs(retainedRange, candidates)
	if err != nil {
		return nil, err
	}
	retired, err := resolveContextCompressionRetirements(action, candidates)
	if err != nil {
		return nil, err
	}
	hasSummarySource := false
	for _, candidate := range candidates {
		if candidate.Kind == "" && strings.TrimSpace(candidate.PromptText) != "" {
			hasSummarySource = true
			break
		}
	}
	if summary == "" && (hasSummarySource || len(retired) > 0) {
		return nil, fmt.Errorf("summary 不能为空；淘汰了技能或证据时，要在末尾写明本次淘汰了什么")
	}
	if strings.Contains(summary, aiprojection.Nonce()) {
		return nil, fmt.Errorf("timeline compression returned a projection control token")
	}
	selection := &aicommon.TimelineCompressionSelection{
		Summary: summary, RetainedRange: strings.TrimSpace(retainedRange), RetainedIDs: retained,
		RetiredPromotedEntries: retired, MemoryEntities: aicommon.ParseTimelineCompressionMemoryEntities(action),
	}
	if maxSummaryTokens > 0 {
		var retainedText strings.Builder
		retainedText.WriteString(summary)
		selected := make(map[int64]bool, len(retained))
		for _, id := range retained {
			selected[id] = true
		}
		for _, candidate := range candidates {
			if selected[candidate.ID] {
				retainedText.WriteByte('\n')
				retainedText.WriteString(candidate.PromptText)
			}
		}
		memoryJSON, _ := json.Marshal(aicommon.ParseTimelineCompressionMemoryEntities(action))
		retainedText.Write(memoryJSON)
		if aicommon.TokenCountExceeds(retainedText.String(), maxSummaryTokens) {
			return nil, fmt.Errorf("timeline compression summary and retained items exceed safety limit %d", maxSummaryTokens)
		}
	}
	return selection, nil
}

type contextCompressionRequest struct {
	throughID *int64
	itemIDs   []int64
}

func (r *ReActLoop) prepareContextCompression(action *aicommon.Action, request ...contextCompressionRequest) (*aicommon.TimelineCompressionSelection, error) {
	if r == nil {
		return nil, fmt.Errorf("context compression requires a loop")
	}
	timeline := timelineFromConfig(r.config)
	if timeline == nil {
		return nil, fmt.Errorf("context compression requires a timeline")
	}
	var through []int64
	var observedIDs []int64
	if len(request) > 0 {
		observedIDs = request[0].itemIDs
		if request[0].throughID != nil {
			through = []int64{*request[0].throughID}
		}
	}
	candidates, err := timeline.CompressionCandidates(through...)
	if err != nil {
		return nil, err
	}
	if observedIDs != nil {
		observed := make(map[int64]bool, len(observedIDs))
		for _, id := range observedIDs {
			observed[id] = true
		}
		visible := candidates[:0]
		for _, candidate := range candidates {
			if observed[candidate.ID] {
				visible = append(visible, candidate)
			}
		}
		candidates = visible
	}
	selection, err := r.parseContextCompressionSelection(action, candidates, aicommon.TimelineCompressionMaxSummaryTokens)
	if err == nil && len(through) > 0 {
		selection.ThroughID = &through[0]
	}
	if err == nil {
		selection.ObservedIDs = observedIDs
	}
	return selection, err
}

func parseContextCompressionRetainedIDs(text string, candidates []aicommon.TimelineCompressionCandidate) ([]int64, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	eligible := make(map[int64]bool)
	for _, candidate := range candidates {
		if candidate.Kind == "" && candidate.PromptText != "" {
			eligible[candidate.ID] = true
		}
	}
	selected := make(map[int64]bool)
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		first, last, ranged := strings.Cut(part, "-")
		if !ranged {
			last = first
		}
		start, startErr := strconv.ParseInt(first, 10, 64)
		end, endErr := strconv.ParseInt(last, 10, 64)
		if startErr != nil || endErr != nil || start <= 0 || end < start ||
			strings.HasPrefix(first, "+") || strings.HasPrefix(last, "+") || !eligible[start] || !eligible[end] {
			return nil, fmt.Errorf("retain_timeline 范围 %q 无效；使用动态区列出的普通历史 ID，例如 12,18-23，起止 ID 都必须存在且递增", part)
		}
		for id := range eligible {
			if id >= start && id <= end {
				selected[id] = true
			}
		}
	}
	ids := make([]int64, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func resolveContextCompressionRetirements(action *aicommon.Action, candidates []aicommon.TimelineCompressionCandidate) ([]int64, error) {
	skills := action.GetStringSlice("retire_skill_names")
	evidence := action.GetStringSlice("retire_evidence_ids")
	knownSkills := map[string][]int64{}
	knownEvidence := map[string][]int64{}
	for _, candidate := range candidates {
		switch candidate.Kind {
		case aicommon.TimelinePromotedKindSkillContent:
			if name := contextCompressionSkillName(candidate); name != "" {
				knownSkills[name] = append(knownSkills[name], candidate.ID)
			}
		case aicommon.TimelinePromotedKindEvidence:
			if name := strings.TrimSpace(candidate.Key); name != "" {
				knownEvidence[name] = append(knownEvidence[name], candidate.ID)
			}
		}
	}
	var retired []int64
	seen := map[int64]bool{}
	add := func(ids []int64) {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				retired = append(retired, id)
			}
		}
	}
	for _, name := range skills {
		ids, ok := knownSkills[name]
		if !ok {
			return nil, fmt.Errorf("retire_skill_names 有未知技能 %q，只能使用动态区列出的技能名", name)
		}
		add(ids)
	}
	for _, name := range evidence {
		ids, ok := knownEvidence[name]
		if !ok {
			return nil, fmt.Errorf("retire_evidence_ids 有未知证据 %q，只能使用动态区列出的证据 id", name)
		}
		add(ids)
	}
	sort.Slice(retired, func(i, j int) bool { return retired[i] < retired[j] })
	return retired, nil
}

func contextCompressionSkillName(candidate aicommon.TimelineCompressionCandidate) string {
	if candidate.Kind != aicommon.TimelinePromotedKindSkillContent {
		return ""
	}
	if candidate.Key == "catalog" {
		return "catalog"
	}
	name := candidate.Key
	if index := strings.IndexByte(name, 0); index >= 0 {
		name = name[:index]
	}
	return strings.TrimSpace(name)
}
