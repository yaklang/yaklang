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
	"github.com/yaklang/yaklang/common/log"
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

// Reuse the generic warning stream; the localized payload identifies the source.
const contextCompressionWarningNodeID = "warning"

// Selection mistakes can be corrected by the model; runtime failures cannot.
type contextCompressionSelectionError struct{ err error }

func (e *contextCompressionSelectionError) Error() string { return e.err.Error() }
func (e *contextCompressionSelectionError) Unwrap() error { return e.err }

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

func (r *ReActLoop) emitContextCompressionAdvisoryLimit(actualTokens, limitTokens int) {
	if r == nil || r.GetEmitter() == nil {
		return
	}
	messageZh := fmt.Sprintf(
		"上下文压缩结果超过建议阈值（实际 %d tokens，建议 %d tokens），已继续处理。\n摘要和保留历史不会因该建议阈值被截断。",
		actualTokens, limitTokens,
	)
	messageEn := fmt.Sprintf(
		"Context compression exceeded the advisory limit (%d actual tokens, %d recommended tokens); processing continued.\nThe summary and retained history will not be truncated because of this advisory threshold.",
		actualTokens, limitTokens,
	)
	message := messageZh
	language := ""
	if r.config != nil {
		if languageProvider, ok := r.config.(interface{ GetLanguage() string }); ok {
			language = languageProvider.GetLanguage()
		}
		if language == "" {
			language = r.config.GetConfigString("language")
		}
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		message = messageEn
	}
	taskID := ""
	if task := r.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	if _, err := r.GetEmitter().EmitDefaultStreamEvent(
		contextCompressionWarningNodeID,
		strings.NewReader(message),
		taskID,
	); err != nil {
		log.Warnf("emit context compression advisory warning: %v", err)
	}
}

func (r *ReActLoop) parseContextCompressionSelection(action *aicommon.Action, candidates []aicommon.TimelineCompressionCandidate, maxSummaryTokens int) (*aicommon.TimelineCompressionSelection, error) {
	if action == nil {
		return nil, fmt.Errorf("context compression requires an action")
	}
	rawSummary, _ := action.LookupCanonicalParam("summary")
	summary := strings.TrimSpace(contextCompressionTextValue(rawSummary, "\n"))
	rawRange, _ := action.LookupCanonicalParam("retain_timeline")
	retainedRange := strings.TrimSpace(contextCompressionTextValue(rawRange, ","))
	retained := parseContextCompressionRetainedIDs(retainedRange, candidates)
	retired := resolveContextCompressionRetirements(action, candidates)
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
		if actualTokens := aicommon.MeasureTokens(retainedText.String()); actualTokens > maxSummaryTokens {
			r.emitContextCompressionAdvisoryLimit(actualTokens, maxSummaryTokens)
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
	if err != nil {
		return nil, &contextCompressionSelectionError{err: err}
	}
	if len(through) > 0 {
		selection.ThroughID = &through[0]
	}
	selection.ObservedIDs = observedIDs
	return selection, nil
}

func parseContextCompressionRetainedIDs(text string, candidates []aicommon.TimelineCompressionCandidate) []int64 {
	if strings.TrimSpace(text) == "" {
		return nil
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
		if part == "" {
			continue
		}
		first, last, ranged := strings.Cut(part, "-")
		if !ranged {
			last = first
		}
		start, startErr := strconv.ParseInt(strings.TrimSpace(first), 10, 64)
		end, endErr := strconv.ParseInt(strings.TrimSpace(last), 10, 64)
		if startErr != nil || endErr != nil || start <= 0 || end < start ||
			strings.HasPrefix(strings.TrimSpace(first), "+") || strings.HasPrefix(strings.TrimSpace(last), "+") {
			// Optional selections are best-effort. Ignore only the malformed
			// segment and keep any other usable IDs from the same action.
			continue
		}
		// A selected ID does not have to be ordinary history. It can identify
		// protected user input, a promoted item, or an internal entry that this
		// action cannot retire. Intersect with ordinary candidates and retain the
		// actual compressible items covered by the selection.
		for id := range eligible {
			if id >= start && id <= end {
				selected[id] = true
			}
		}
	}
	if len(selected) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func resolveContextCompressionRetirements(action *aicommon.Action, candidates []aicommon.TimelineCompressionCandidate) []int64 {
	skills := contextCompressionStringArray(action, "retire_skill_names")
	evidence := contextCompressionStringArray(action, "retire_evidence_ids")
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
		name = strings.TrimSpace(name)
		ids, ok := knownSkills[name]
		if !ok {
			log.Debugf("ignore unknown context compression skill %q", name)
			continue
		}
		add(ids)
	}
	for _, name := range evidence {
		name = strings.TrimSpace(name)
		ids, ok := knownEvidence[name]
		if !ok {
			log.Debugf("ignore unknown context compression evidence %q", name)
			continue
		}
		add(ids)
	}
	sort.Slice(retired, func(i, j int) bool { return retired[i] < retired[j] })
	return retired
}

func contextCompressionStringArray(action *aicommon.Action, name string) []string {
	raw, _ := action.LookupCanonicalParam(name)
	return contextCompressionStringValues(raw)
}

func contextCompressionStringValues(raw any) []string {
	switch values := raw.(type) {
	case []string:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				result = append(result, value)
			}
		}
		return result
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			result = append(result, contextCompressionStringValues(value)...)
		}
		return result
	case string:
		text := strings.TrimSpace(values)
		if text == "" {
			return nil
		}
		var decoded []any
		if json.Unmarshal([]byte(text), &decoded) == nil {
			return contextCompressionStringValues(decoded)
		}
		parts := strings.FieldsFunc(text, func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r'
		})
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
		return result
	case nil:
		return nil
	default:
		if text := strings.TrimSpace(fmt.Sprint(values)); text != "" {
			return []string{text}
		}
		return nil
	}
}

func contextCompressionTextValue(raw any, separator string) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return value
	case []byte:
		return string(value)
	case json.Number:
		return value.String()
	case []string:
		return strings.Join(value, separator)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := strings.TrimSpace(contextCompressionTextValue(item, separator)); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, separator)
	case map[string]any:
		for _, key := range []string{"text", "content", "summary", "value", "id"} {
			if item, ok := value[key]; ok {
				return contextCompressionTextValue(item, separator)
			}
		}
		if encoded, err := json.Marshal(value); err == nil {
			return string(encoded)
		}
		return fmt.Sprint(value)
	default:
		return fmt.Sprint(value)
	}
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
