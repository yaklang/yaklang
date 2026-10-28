package aicommon

import (
	"fmt"
	"sort"
	"strings"
)

// TimelineCompressionCandidate is the immutable candidate view exposed to the
// main-loop action. Parsing and selection remain in the action layer.
type TimelineCompressionCandidate struct {
	ID         int64
	PromptText string
	Kind       string
	Key        string
}

// TimelineCompressionSelection is the fully parsed action result. Timeline
// applies these IDs only through the request's rendered history boundary.
type TimelineCompressionSelection struct {
	// Nil is the compatibility path for direct callers applying to current history.
	ThroughID              *int64
	ObservedIDs            []int64
	Summary                string
	RetainedRange          string
	RetainedIDs            []int64
	RetiredPromotedEntries []int64
	MemoryEntities         []any
}

// CompressionBoundary captures identities, not bodies. A result may reserve a
// low ID before execution and arrive after newer entries have already rendered.
// An optional through ID caps both the reported boundary and its item IDs.
func (m *Timeline) CompressionBoundary(throughID ...int64) (int64, []int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	through := m.getMaxIDLocked()
	if len(throughID) > 0 && throughID[0] >= 0 {
		through = min(throughID[0], through)
	}
	return through, m.compressionItemIDsLocked(through)
}

func (m *Timeline) compressionItemIDsLocked(throughID ...int64) []int64 {
	through := m.getMaxIDLocked()
	if len(throughID) > 0 && throughID[0] >= 0 {
		through = min(throughID[0], through)
	}
	ids := make([]int64, 0, m.idToTimelineItem.Len())
	for _, id := range m.idToTimelineItem.Keys() {
		if id > through {
			continue
		}
		if item, ok := m.idToTimelineItem.Get(id); ok && item != nil && !item.deleted {
			ids = append(ids, id)
		}
	}
	return ids
}

// RenderCompressionGuide reads the current timeline and returns the dynamic
// compression guide. It does not reserve or retain a snapshot.
func (m *Timeline) RenderCompressionGuide(options TimelineCompressionOptions) (string, error) {
	if m == nil {
		return "", nil
	}
	options = m.prepareCompressionOptions(options)
	if err := options.Context.Err(); err != nil {
		return "", err
	}
	m.mu.RLock()
	through := m.getMaxIDLocked()
	if options.ThroughID != nil {
		through = *options.ThroughID
	}
	snapshot, err := m.captureCompressionSnapshotThroughLocked(through)
	m.mu.RUnlock()
	if err != nil {
		return "", err
	}
	if err := prepareCompressionSnapshot(snapshot); err != nil && len(snapshot.ExactItemIDs) == 0 {
		return "", err
	}
	return renderCompressionGuide(snapshot, options), nil
}

// CompressionCandidates reads the request's history range for action translation.
// The returned slice is detached and is not retained by Timeline.
func (m *Timeline) CompressionCandidates(throughID ...int64) ([]TimelineCompressionCandidate, error) {
	if m == nil {
		return nil, fmt.Errorf("timeline is nil")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	through := m.getMaxIDLocked()
	if len(throughID) > 0 {
		through = throughID[0]
	}
	snapshot, err := m.captureCompressionSnapshotThroughLocked(through)
	if err != nil {
		return nil, err
	}
	candidates := make([]TimelineCompressionCandidate, 0, len(snapshot.Items)+len(snapshot.PromotedItems))
	for _, item := range snapshot.Items {
		candidates = append(candidates, TimelineCompressionCandidate{
			ID: item.ID, PromptText: item.PromptText,
		})
	}
	for _, item := range snapshot.PromotedItems {
		candidates = append(candidates, TimelineCompressionCandidate{
			ID: item.ID, PromptText: item.PromptText, Kind: item.Kind, Key: item.Key,
		})
	}
	return candidates, nil
}

// ApplyCompressionSelection applies a parsed action through its request boundary.
// It captures a local memory source without retaining a cross-request snapshot.
func (m *Timeline) ApplyCompressionSelection(selection *TimelineCompressionSelection, requestPrompt string, response ...TrajectoryResponseReceipt) (*TimelineCompressionResult, error) {
	if m == nil {
		return nil, fmt.Errorf("timeline is nil")
	}
	if selection == nil {
		return nil, fmt.Errorf("compression selection is nil")
	}
	if config := m.compressionCallerConfig(); config != nil {
		if ctx := config.GetContext(); ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
	}
	m.mu.Lock()
	trajectoryConfig := m.trajectoryConfig
	var responseSnapshot TrajectoryResponseReceipt
	if len(response) > 0 {
		responseSnapshot = response[0]
	}
	result, notify, err := m.applyCompressionLocked(selection, requestPrompt, trajectoryConfig, responseSnapshot)
	m.mu.Unlock()
	if notify != nil {
		notify()
	}
	return result, err
}

func renderCompressionGuide(snapshot *timelineCompressionSnapshot, options TimelineCompressionOptions) string {
	skills := map[string]struct{}{}
	hasEvidence := false
	for _, item := range snapshot.PromotedItems {
		switch item.Kind {
		case TimelinePromotedKindSkillContent:
			if name := compressionSkillChoiceName(item); name != "" {
				skills[name] = struct{}{}
			}
		case TimelinePromotedKindEvidence:
			if strings.TrimSpace(item.Key) != "" {
				hasEvidence = true
			}
		}
	}
	var b strings.Builder
	b.WriteString("# 本轮只压缩上下文\n")
	b.WriteString("本轮不是执行任务。上文各段及本轮只读状态就是主循环的真实上下文。不要调用业务工具，不要修改 TODO，不要回答用户。只能选择 compress_context，且只选择一次。\n")
	b.WriteString("压缩方法见前面的「压缩 Timeline」指引。timeline 正文已经在前缀里，这里只补充整份 prompt 的容量，以及可以整份淘汰的技能名。\n\n")
	if options.PromptTokens > 0 {
		fmt.Fprintf(&b, "当前整份主循环 prompt：%d tokens。\n", options.PromptTokens)
	}
	if options.TargetTokens > 0 {
		fmt.Fprintf(&b, "整份 prompt 压缩目标：接近 %d tokens（限制值的六分之一）。关键信息保留良好优先于继续压低。\n", options.TargetTokens)
		if options.PromptTokens > options.TargetTokens {
			fmt.Fprintf(&b, "目前高出目标：%d tokens。\n", options.PromptTokens-options.TargetTokens)
		}
	}
	b.WriteString("\n# 普通历史 ID 从哪里看\n")
	b.WriteString("不要在这里重复列出普通历史。retain_timeline 使用上文 Timeline Memory 的 Frozen Prefix 和 Open Tail 中每条普通历史标题里的 `[item_id:N]` 数字 N；标题格式是 `HH:MM:SS [类型] [item_id:N]`，不要把时间、工具调用 id 或块标签当作历史 ID。\n")
	b.WriteString("可以写单个 ID 或闭区间，多个散落段用逗号组合，例如 \"3-8,12,20-26,37,41-45\"。空字符串表示全部由 summary 概括。范围只选择普通历史，不影响夹在其中的技能、证据或用户输入。一条工具调用及其结果按整条保留，不拆开。\n")
	if len(snapshot.Items) == 0 {
		b.WriteString("（没有可概括的普通历史）\n")
	}
	b.WriteString("\n# 可淘汰的技能\n")
	b.WriteString("名字与 semi-dynamic 里 `=== Skill: 名字 ===` 一致。retire_skill_names 写要整份去掉的名字；同一名字下的分页和 metadata 会一起去掉。空数组表示全部保留。\n")
	if len(skills) == 0 {
		b.WriteString("（没有可淘汰的技能）\n")
	}
	for _, skill := range sortedCompressionChoices(skills) {
		b.WriteString(skill)
		b.WriteByte('\n')
	}
	b.WriteString("\n# 证据 id 从哪里看\n")
	b.WriteString("不要在这里重复列出证据。retire_evidence_ids 使用上文「已知观测（Evidence）」里 `[id: ...]` 的 id。只写确认不再影响下一步的 id。空数组表示全部保留。\n")
	if !hasEvidence {
		b.WriteString("（没有可淘汰的证据）\n")
	}
	b.WriteString("\nRecently Used Tools、Session User Input History、TODO、任务指令和目录都不能删除。\n")
	return strings.TrimRight(b.String(), "\n")
}

func sortedCompressionChoices(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func compressionSkillChoiceName(item timelineCompressionPromotedItem) string {
	if item.Kind != TimelinePromotedKindSkillContent {
		return ""
	}
	if item.Key == skillCatalogKey {
		return "catalog"
	}
	name := item.Key
	if index := strings.IndexByte(name, 0); index >= 0 {
		name = name[:index]
	}
	return strings.TrimSpace(name)
}
