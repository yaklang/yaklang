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
// applies these IDs to the timeline as it exists at apply time.
type TimelineCompressionSelection struct {
	Summary                string
	RetainedRange          string
	RetainedIDs            []int64
	RetiredPromotedEntries []int64
	MemoryEntities         []any
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
	snapshot, err := m.captureCompressionSnapshotLocked()
	m.mu.RUnlock()
	if err != nil {
		return "", err
	}
	if err := prepareCompressionSnapshot(snapshot); err != nil && len(snapshot.ExactItemIDs) == 0 {
		return "", err
	}
	return renderCompressionGuide(snapshot, options), nil
}

// CompressionCandidates reads the current timeline for action translation.
// The returned slice is detached and is not retained by Timeline.
func (m *Timeline) CompressionCandidates() ([]TimelineCompressionCandidate, error) {
	if m == nil {
		return nil, fmt.Errorf("timeline is nil")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot, err := m.captureCompressionSnapshotLocked()
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

// ApplyCompressionSelection applies an already parsed action to the timeline
// as it exists now. It does not compare the timeline with an earlier read.
func (m *Timeline) ApplyCompressionSelection(selection *TimelineCompressionSelection, requestPrompt string) (*TimelineCompressionResult, error) {
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
	result, notify, err := m.applyCompressionLocked(selection, requestPrompt)
	m.mu.Unlock()
	if notify != nil {
		notify()
	}
	return result, err
}

func renderCompressionGuide(snapshot *timelineCompressionSnapshot, options TimelineCompressionOptions) string {
	skills := map[string]struct{}{}
	evidence := map[string]struct{}{}
	for _, item := range snapshot.PromotedItems {
		switch item.Kind {
		case TimelinePromotedKindSkillContent:
			if name := compressionSkillChoiceName(item); name != "" {
				skills[name] = struct{}{}
			}
		case TimelinePromotedKindEvidence:
			if name := strings.TrimSpace(item.Key); name != "" {
				evidence[name] = struct{}{}
			}
		}
	}
	var b strings.Builder
	b.WriteString("# 本轮只压缩上下文\n")
	b.WriteString("本轮不是执行任务。上文各段及本轮只读状态就是主循环的真实上下文。不要调用业务工具，不要修改 TODO，不要回答用户。只能选择 compress_context，且只选择一次。\n")
	b.WriteString("压缩方法见前面的「压缩 Timeline」指引。以下列出整份主循环 prompt 的容量和允许选择的 ID、技能名、证据 id。\n\n")
	if options.PromptTokens > 0 {
		fmt.Fprintf(&b, "当前整份主循环 prompt：%d tokens。\n", options.PromptTokens)
	}
	if options.TargetTokens > 0 {
		fmt.Fprintf(&b, "整份 prompt 压缩目标：接近 %d tokens（限制值的六分之一）。关键信息保留良好优先于继续压低。\n", options.TargetTokens)
		if options.PromptTokens > options.TargetTokens {
			fmt.Fprintf(&b, "目前高出目标：%d tokens。\n", options.PromptTokens-options.TargetTokens)
		}
	}
	b.WriteString("\n# 可保留的普通历史\n")
	b.WriteString("retain_timeline 支持多个散落的 ID 段和单个 ID，用逗号组合，例如 \"3-8,12,20-26,37,41-45\"；空字符串表示全部由 summary 概括。范围只选择下列普通历史，不影响夹在其中的技能、证据或用户输入。条目按整条保留，不拆分工具调用及其结果。正文片段仅用于定位，无需复制。\n")
	if len(snapshot.Items) == 0 {
		b.WriteString("（没有可概括的普通历史）\n")
	}
	for _, item := range snapshot.Items {
		if item.PromptText != "" {
			fmt.Fprintf(&b, "ID=%d %s\n", item.ID, compressionIndexLocator(item.PromptText))
		}
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
	b.WriteString("\n# 可淘汰的证据\n")
	b.WriteString("id 与「已知观测（Evidence）」里的 id 一致。retire_evidence_ids 只写确认不再影响下一步的 id。空数组表示全部保留。\n")
	if len(evidence) == 0 {
		b.WriteString("（没有可淘汰的证据）\n")
	}
	for _, id := range sortedCompressionChoices(evidence) {
		b.WriteString(id)
		b.WriteByte('\n')
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
