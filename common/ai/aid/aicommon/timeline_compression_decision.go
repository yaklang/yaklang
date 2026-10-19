package aicommon

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// TimelineCompressionAnchor is one ordinary history entry the model can keep
// by copying Token. Token is taken from the visible timeline text, not a bare ID.
type TimelineCompressionAnchor struct {
	Token string
	ID    int64
}

// TimelineCompressionChoice is one promoted item the model can retire by the
// name or evidence id already shown in the main-loop prompt.
type TimelineCompressionChoice struct {
	Name string
	Kind string
	IDs  []int64
}

// TimelineCompressionView is the dynamic-section brief for one compression action.
type TimelineCompressionView struct {
	DynamicGuide string
	Anchors      []TimelineCompressionAnchor
	Skills       []TimelineCompressionChoice
	Evidence     []TimelineCompressionChoice
}

// CompressionDecision is one code-triggered compression after the lossless fold.
// The reservation stays held until Commit or Abort.
type CompressionDecision struct {
	timeline *Timeline
	snapshot *timelineCompressionSnapshot
	options  TimelineCompressionOptions
	usage    timelinePromptUsage
	view     TimelineCompressionView
	anchors  map[string]int64
	skills   map[string][]int64
	evidence map[string][]int64
	output   *timelineCompressionOutput
	commits  []*timelineCompressionCommit
	finished bool
}

func (d *CompressionDecision) View() TimelineCompressionView {
	if d == nil {
		return TimelineCompressionView{}
	}
	return d.view
}

// BeginCompressionDecision opens one model compression after the caller has
// already folded and confirmed the whole prompt is still above the target.
func (m *Timeline) BeginCompressionDecision(options TimelineCompressionOptions) (*CompressionDecision, error) {
	if m == nil {
		return nil, nil
	}
	options = m.prepareCompressionOptions(options)
	if options.MaxInputTokens == 0 {
		options.MaxInputTokens = TimelineCompressionMaxInputTokens
	}
	if options.MaxSummaryTokens == 0 {
		options.MaxSummaryTokens = TimelineCompressionMaxSummaryTokens
	}
	if err := m.awaitCompression(options); err != nil {
		return nil, err
	}
	decision := &CompressionDecision{timeline: m, options: options}
	usage := timelinePromptUsage{Limit: m.GetTimelineContentLimit()}
	if usage.Limit <= 0 {
		usage.Limit = 300 * 1024
	}
	decision.usage = usage
	options.reportProgress(TimelineCompressionOptimizing)
	m.mu.Lock()
	snapshot, err := m.captureCompressionSnapshotLocked()
	if err != nil {
		m.mu.Unlock()
		decision.Abort(err)
		return nil, err
	}
	m.compressionSnapshot = snapshot
	snapshot.TargetTokens = max(int64(1), usage.Limit/timelineCompressionTargetDivisor)
	snapshot.Context = options.Context
	blocks, _, budgetErr := m.captureCompressionBudgetBlocksLocked(options.PromptOptions)
	m.mu.Unlock()
	if budgetErr != nil {
		decision.Abort(budgetErr)
		return nil, budgetErr
	}
	snapshot.PromptBudget = newCompressionPromptBudget(blocks, usage.Limit, snapshot.TargetTokens)
	snapshot.RetainedContext = make(map[string]string, len(options.RetainedContext))
	for key, value := range options.RetainedContext {
		snapshot.RetainedContext[key] = value
	}
	if err := prepareCompressionSnapshot(snapshot); err != nil && len(snapshot.ExactItemIDs) == 0 {
		decision.Abort(err)
		return nil, err
	}
	if err := options.Context.Err(); err != nil {
		decision.Abort(err)
		return nil, err
	}
	if err := m.sessionMemory.waitForModelTail(options.Context, func() {
		options.reportProgress(TimelineCompressionMemory)
	}); err != nil {
		decision.Abort(err)
		return nil, err
	}
	snapshot.SessionMemoryCandidates = m.sessionMemory.snapshot()
	decision.snapshot = snapshot
	decision.view = decision.buildView()
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
	decision := &CompressionDecision{timeline: m, options: options}
	folded, err := m.foldTimelineWithoutAI(ctx)
	if folded != nil {
		decision.commits = append(decision.commits, folded)
	}
	decision.finish(nil)
	return err
}

func (d *CompressionDecision) Abort(err error) {
	if d == nil || d.finished {
		return
	}
	if err != nil && d.timeline != nil {
		d.timeline.recordCompressionOutcome(d.options, d.usage, err)
	}
	d.finish(nil)
}

func (d *CompressionDecision) finish(extra *timelineCompressionCommit) {
	if d == nil || d.finished {
		return
	}
	d.finished = true
	if extra != nil {
		d.commits = append(d.commits, extra)
	}
	d.timeline.finishCompression(d.commits)
}

// Validate checks one model action against the catalog shown in this decision.
// It does not commit. Unknown anchors, skill names, or evidence ids are errors
// so the action can retry with guidance instead of deleting the wrong item.
func (d *CompressionDecision) Validate(action *Action) error {
	if d == nil || d.snapshot == nil || d.finished {
		return fmt.Errorf("compression decision is not open")
	}
	if action == nil || (action.Name() != TimelineCompressionAction && action.Name() != "timeline-summary") {
		return fmt.Errorf("必须输出 compress_timeline")
	}
	rawSummary, _ := action.LookupCanonicalParam("summary")
	summary, ok := rawSummary.(string)
	if !ok {
		return fmt.Errorf("summary 必须是字符串")
	}
	summary = strings.TrimSpace(summary)
	retained, err := d.lookupAnchors(action)
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
	output := parseTimelineCompressionOutput(action, d.snapshot, summary)
	output.RetainedIDs = retained
	output.RetainedRange = strings.Join(d.anchorTokens(retained), "\n")
	output.RetiredPromotedEntries = retired
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
	d.timeline.recordCompressionOutcome(d.options, d.usage, nil)
	d.finish(commit)
	if commit == nil {
		return nil, nil
	}
	return commit.Result, nil
}

func (d *CompressionDecision) buildView() TimelineCompressionView {
	view := TimelineCompressionView{}
	d.anchors = map[string]int64{}
	d.skills = map[string][]int64{}
	d.evidence = map[string][]int64{}
	seenAnchor := map[string]int{}
	for _, item := range d.snapshot.Items {
		token := compressionAnchorToken(item)
		if token == "" {
			continue
		}
		seenAnchor[token]++
		if seenAnchor[token] > 1 {
			token = fmt.Sprintf("%s ·%d", token, seenAnchor[token])
		}
		d.anchors[token] = item.ID
		view.Anchors = append(view.Anchors, TimelineCompressionAnchor{Token: token, ID: item.ID})
	}
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
	for _, name := range skillOrder {
		ids := append([]int64(nil), d.skills[name]...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		d.skills[name] = ids
		view.Skills = append(view.Skills, TimelineCompressionChoice{Name: name, Kind: TimelinePromotedKindSkillContent, IDs: ids})
	}
	for _, name := range evidenceOrder {
		ids := append([]int64(nil), d.evidence[name]...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		d.evidence[name] = ids
		view.Evidence = append(view.Evidence, TimelineCompressionChoice{Name: name, Kind: TimelinePromotedKindEvidence, IDs: ids})
	}
	view.DynamicGuide = renderCompressionDecisionGuide(d.snapshot, view)
	d.view = view
	return view
}

func renderCompressionDecisionGuide(snapshot *timelineCompressionSnapshot, view TimelineCompressionView) string {
	var b strings.Builder
	b.WriteString("# 本轮只压缩上下文\n")
	b.WriteString("本轮不是执行任务。上面各段就是主循环下一轮会看到的真实上下文。不要调用工具，不要修改 TODO，不要回答用户。只输出 compress_timeline。\n")
	b.WriteString("压缩方法在 PERSISTENT 指令里。这里只给出容量和允许点名的记号；记号必须从下面原样复制，不要发明，也不要改写。\n\n")
	if snapshot != nil {
		budget := snapshot.PromptBudget
		target := snapshot.TargetTokens
		if target == 0 {
			target = budget.Target
		}
		ordinary, promoted := compressionCapacityParts(budget)
		if budget.Current > 0 {
			fmt.Fprintf(&b, "当前 Timeline：%d tokens。\n", budget.Current)
		}
		if target > 0 {
			fmt.Fprintf(&b, "压缩目标：接近 %d tokens。目标不是越低越好；关键信息保留良好优先于继续压低。\n", target)
		}
		if budget.Current > 0 && target > 0 && budget.Current > target {
			fmt.Fprintf(&b, "目前高出目标：%d tokens。只处理能补上这个差额的弱关联材料。\n", budget.Current-target)
		}
		if ordinary > 0 || promoted > 0 {
			fmt.Fprintf(&b, "可优化部分：普通历史 %d tokens，技能与证据 %d tokens。工具 Schema 和用户输入不在可删除范围内。\n", ordinary, promoted)
		}
		if query := strings.TrimSpace(compressionRetainedText(snapshot, "user_query")); query != "" {
			b.WriteString("\n当前任务目标只用于判断关联，不要写进 summary：\n")
			b.WriteString(query)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n# 可保留的普通历史\n")
	b.WriteString("每一行都是 Frozen Prefix 或 Open Tail 里能对上的一条原文。retain_timeline 原样复制需要留下的整行；空数组表示这些普通历史都由 summary 概括。\n")
	if len(view.Anchors) == 0 {
		b.WriteString("（没有可概括的普通历史）\n")
	}
	for _, anchor := range view.Anchors {
		b.WriteString(anchor.Token)
		b.WriteByte('\n')
	}
	b.WriteString("\n# 可淘汰的技能\n")
	b.WriteString("名字与 semi-dynamic 里 `=== Skill: 名字 ===` 一致。retire_skill_names 写要整份去掉的名字；同一名字下的分页和 metadata 会一起去掉。空数组表示全部保留。\n")
	if len(view.Skills) == 0 {
		b.WriteString("（没有可淘汰的技能）\n")
	}
	for _, skill := range view.Skills {
		b.WriteString(skill.Name)
		b.WriteByte('\n')
	}
	b.WriteString("\n# 可淘汰的证据\n")
	b.WriteString("id 与「已知观测（Evidence）」里的 id 一致。retire_evidence_ids 只写确认不再影响下一步的 id。空数组表示全部保留。\n")
	if len(view.Evidence) == 0 {
		b.WriteString("（没有可淘汰的证据）\n")
	}
	for _, evidence := range view.Evidence {
		b.WriteString(evidence.Name)
		b.WriteByte('\n')
	}
	b.WriteString("\nRecently Used Tools、Session User Input History、TODO、任务指令和目录都不能删除。\n")
	return strings.TrimRight(b.String(), "\n")
}

func compressionAnchorToken(item timelineCompressionSnapshotItem) string {
	body := item.PromptText
	header := body
	if index := strings.IndexByte(body, '\n'); index >= 0 {
		header, body = body[:index], body[index+1:]
	}
	kind := "entry"
	if start := strings.LastIndex(header, "["); start >= 0 {
		if end := strings.Index(header[start:], "]"); end > 1 {
			kind = strings.TrimSpace(header[start+1 : start+end])
		}
	}
	locator := compressionIndexLocator(body)
	if locator == "" {
		return ""
	}
	clock := time.UnixMilli(item.Timestamp).Format("15:04:05")
	return clock + " [" + kind + "] " + locator
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

func (d *CompressionDecision) lookupAnchors(action *Action) ([]int64, error) {
	tokens, err := compressionStringList(action, "retain_timeline")
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	selected := make([]int64, 0, len(tokens))
	seen := map[int64]bool{}
	for _, token := range tokens {
		id, ok := d.anchors[token]
		if !ok {
			return nil, fmt.Errorf("retain_timeline 有无法对应的记号 %q，必须原样复制动态区列出的整行", token)
		}
		if !seen[id] {
			seen[id] = true
			selected = append(selected, id)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i] < selected[j] })
	return selected, nil
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

func (d *CompressionDecision) anchorTokens(ids []int64) []string {
	wanted := map[int64]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	var tokens []string
	for _, anchor := range d.view.Anchors {
		if wanted[anchor.ID] {
			tokens = append(tokens, anchor.Token)
		}
	}
	return tokens
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
