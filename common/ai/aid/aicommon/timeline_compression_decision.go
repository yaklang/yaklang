package aicommon

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// CompressionDecision is one code-triggered compression after the lossless fold.
// The reservation stays held until Commit or Abort.
type CompressionDecision struct {
	timeline *Timeline
	snapshot *timelineCompressionSnapshot
	options  TimelineCompressionOptions
	guide    string
	skills   map[string][]int64
	evidence map[string][]int64
	output   *timelineCompressionOutput
	finished bool
}

func (d *CompressionDecision) DynamicGuide() string {
	if d == nil {
		return ""
	}
	return d.guide
}

// BeginCompressionDecision opens one model compression after the caller has
// already folded and confirmed the whole prompt is still above the target.
func (m *Timeline) BeginCompressionDecision(options TimelineCompressionOptions) (*CompressionDecision, error) {
	if m == nil {
		return nil, nil
	}
	options = m.prepareCompressionOptions(options)
	if options.MaxSummaryTokens == 0 {
		options.MaxSummaryTokens = TimelineCompressionMaxSummaryTokens
	}
	if err := m.awaitCompression(options); err != nil {
		return nil, err
	}
	decision := &CompressionDecision{timeline: m, options: options}
	options.reportProgress(TimelineCompressionOptimizing)
	m.mu.Lock()
	snapshot, err := m.captureCompressionSnapshotLocked()
	if err != nil {
		m.mu.Unlock()
		decision.Abort()
		return nil, err
	}
	m.compressionSnapshot = snapshot
	snapshot.Context = options.Context
	m.mu.Unlock()
	if err := prepareCompressionSnapshot(snapshot); err != nil && len(snapshot.ExactItemIDs) == 0 {
		decision.Abort()
		return nil, err
	}
	if err := options.Context.Err(); err != nil {
		decision.Abort()
		return nil, err
	}
	if err := m.sessionMemory.waitForModelTail(options.Context, func() {
		options.reportProgress(TimelineCompressionMemory)
	}); err != nil {
		decision.Abort()
		return nil, err
	}
	snapshot.SessionMemoryCandidates = m.sessionMemory.snapshot()
	decision.snapshot = snapshot
	decision.buildGuide()
	return decision, nil
}

// FoldRedundantForCompression commits one lossless fold and releases the
// reservation. The caller measures the rebuilt prompt before starting AI.
func (m *Timeline) FoldRedundantForCompression(ctx context.Context) error {
	if m == nil {
		return nil
	}
	options := m.prepareCompressionOptions(TimelineCompressionOptions{Context: ctx})
	if err := m.awaitCompression(options); err != nil {
		return err
	}
	folded, err := m.foldTimelineWithoutAI(ctx)
	m.finishCompression([]*timelineCompressionCommit{folded})
	return err
}

func (d *CompressionDecision) Abort() {
	if d == nil || d.finished {
		return
	}
	d.finish(nil)
}

func (d *CompressionDecision) finish(extra *timelineCompressionCommit) {
	if d == nil || d.finished {
		return
	}
	d.finished = true
	d.timeline.finishCompression([]*timelineCompressionCommit{extra})
}

// Validate checks one model action against the catalog shown in this decision.
// It does not commit. Unknown timeline IDs, skill names, or evidence ids are errors
// so the action can retry with guidance instead of deleting the wrong item.
func (d *CompressionDecision) Validate(action *Action) error {
	if d == nil || d.snapshot == nil || d.finished {
		return fmt.Errorf("compression decision is not open")
	}
	d.output = nil
	if action == nil || action.Name() != TimelineCompressionAction {
		return fmt.Errorf("必须输出 compress_timeline")
	}
	rawSummary, _ := action.LookupCanonicalParam("summary")
	summary, ok := rawSummary.(string)
	if !ok {
		return fmt.Errorf("summary 必须是字符串")
	}
	summary = strings.TrimSpace(summary)
	rawRange, _ := action.LookupCanonicalParam("retain_timeline")
	retainedRange, ok := rawRange.(string)
	if !ok {
		return fmt.Errorf("retain_timeline 必须是 ID/范围字符串，例如 12,18-23；全部概括时返回空字符串")
	}
	retained, err := parseCompressionRetainedRange(retainedRange, d.snapshot.Items)
	if err != nil {
		return err
	}
	retired, err := d.lookupRetirements(action)
	if err != nil {
		return err
	}
	if summary == "" && (d.snapshot.hasSummarySource() || len(retired) > 0) {
		return fmt.Errorf("summary 不能为空；淘汰了技能或证据时，要在末尾写明本次淘汰了什么")
	}
	output := &timelineCompressionOutput{
		Summary: summary, RetainedIDs: retained, RetainedRange: strings.TrimSpace(retainedRange),
		RetiredPromotedEntries: retired, MemoryEntities: parseTimelineCompressionMemories(action),
	}
	if err := validateCompressionSelection(output, d.snapshot, d.options); err != nil {
		return err
	}
	d.output = output
	return nil
}

// Commit publishes the selection last accepted by Validate.
func (d *CompressionDecision) Commit() (*TimelineCompressionResult, error) {
	if d == nil || d.output == nil || d.finished {
		return nil, fmt.Errorf("compression decision has no validated selection")
	}
	if err := d.options.Context.Err(); err != nil {
		return nil, err
	}
	publishCompressionModelOutput(d.snapshot, d.output)
	commit, err := d.timeline.commitCompressionRound(d.snapshot, d.output.Summary)
	if err != nil {
		return nil, err
	}
	d.finish(commit)
	if commit == nil {
		return nil, nil
	}
	return commit.Result, nil
}

func (d *CompressionDecision) buildGuide() {
	d.skills = map[string][]int64{}
	d.evidence = map[string][]int64{}
	skillOrder := []string{}
	evidenceOrder := []string{}
	for _, item := range d.snapshot.PromotedItems {
		switch item.Kind {
		case TimelinePromotedKindSkillContent:
			name := compressionSkillChoiceName(item)
			if name == "" {
				continue
			}
			if _, ok := d.skills[name]; !ok {
				skillOrder = append(skillOrder, name)
			}
			d.skills[name] = append(d.skills[name], item.ID)
		case TimelinePromotedKindEvidence:
			name := strings.TrimSpace(item.Key)
			if name == "" {
				continue
			}
			if _, ok := d.evidence[name]; !ok {
				evidenceOrder = append(evidenceOrder, name)
			}
			d.evidence[name] = append(d.evidence[name], item.ID)
		}
	}
	sort.Strings(skillOrder)
	sort.Strings(evidenceOrder)
	d.guide = renderCompressionDecisionGuide(d.snapshot, d.options, skillOrder, evidenceOrder)
}

func renderCompressionDecisionGuide(snapshot *timelineCompressionSnapshot, options TimelineCompressionOptions, skills, evidence []string) string {
	var b strings.Builder
	b.WriteString("# 本轮只压缩上下文\n")
	b.WriteString("本轮不是执行任务。上面各段就是主循环下一轮会看到的真实上下文。不要调用工具，不要修改 TODO，不要回答用户。只输出 compress_timeline。\n")
	b.WriteString("压缩方法见前面的「压缩 Timeline」指引。以下列出整份主循环 prompt 的容量和允许选择的 ID、技能名、证据 id。\n\n")
	if options.PromptTokens > 0 {
		fmt.Fprintf(&b, "当前整份主循环 prompt：%d tokens。\n", options.PromptTokens)
	}
	if options.TargetTokens > 0 {
		fmt.Fprintf(&b, "整份 prompt 压缩目标：接近 %d tokens（限制值的一半）。关键信息保留良好优先于继续压低。\n", options.TargetTokens)
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
	for _, skill := range skills {
		b.WriteString(skill)
		b.WriteByte('\n')
	}
	b.WriteString("\n# 可淘汰的证据\n")
	b.WriteString("id 与「已知观测（Evidence）」里的 id 一致。retire_evidence_ids 只写确认不再影响下一步的 id。空数组表示全部保留。\n")
	if len(evidence) == 0 {
		b.WriteString("（没有可淘汰的证据）\n")
	}
	for _, id := range evidence {
		b.WriteString(id)
		b.WriteByte('\n')
	}
	b.WriteString("\nRecently Used Tools、Session User Input History、TODO、任务指令和目录都不能删除。\n")
	return strings.TrimRight(b.String(), "\n")
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

func (d *CompressionDecision) lookupRetirements(action *Action) ([]int64, error) {
	skills, err := compressionStringList(action, "retire_skill_names")
	if err != nil {
		return nil, err
	}
	evidence, err := compressionStringList(action, "retire_evidence_ids")
	if err != nil {
		return nil, err
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
		ids, ok := d.skills[name]
		if !ok {
			return nil, fmt.Errorf("retire_skill_names 有未知技能 %q，只能使用动态区列出的技能名", name)
		}
		add(ids)
	}
	for _, name := range evidence {
		ids, ok := d.evidence[name]
		if !ok {
			return nil, fmt.Errorf("retire_evidence_ids 有未知证据 %q，只能使用动态区列出的证据 id", name)
		}
		add(ids)
	}
	sort.Slice(retired, func(i, j int) bool { return retired[i] < retired[j] })
	return retired, nil
}

func compressionStringList(action *Action, key string) ([]string, error) {
	if action == nil {
		return nil, nil
	}
	raw, ok := action.LookupCanonicalParam(key)
	if !ok || raw == nil {
		return nil, nil
	}
	var parts []string
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, nil
		}
		parts = strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == ',' })
	case []any:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s 的每一项都必须是字符串", key)
			}
			parts = append(parts, text)
		}
	default:
		return nil, fmt.Errorf("%s 必须是字符串列表", key)
	}
	var cleaned []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return cleaned, nil
}
