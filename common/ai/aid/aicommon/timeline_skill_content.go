package aicommon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// skillContentPayload 是 Skill 正文 item 的载荷。只保留“读到了什么”, 不承载
// 加载成功/失败的业务状态机: 同一文件的不同阅读内容各自追加为独立 item。
//
// Source 仅作为提示信息 (用户指定 / AI 自动 / 普通加载 / 目录), 用于压缩说明与
// 观测, 不再决定渲染分块: 所有技能正文冻结后进入同一个技能块。
//
// 关键词: skillContentPayload, 最小字段, 只加和读, 来源仅提示
type skillContentPayload struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

// Skill 正文来源取值 (与 aiskillloader.SkillContentSource* 保持一致)。
// 此处不 import aiskillloader 以避免无谓耦合, 仅做字面量校验。
const (
	SkillContentSourceAuto   = "auto"
	SkillContentSourceForced = "forced"
	SkillContentSourceManual = "manual"
)

// skillContentKey 标识技能文件, 用于加载时检查完全重复的正文。
// 提升态按技能名累计正文, 不用本 key 覆盖旧阅读内容。
func skillContentKey(name, path string) string {
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if path == "" {
		path = skillMDPathPlaceholder
	}
	return name + "\x00" + path
}

// skillCatalogKey 标识“最相关 SKILL 目录”, 用于写入去重和显式移除。
const skillCatalogKey = "__skill_catalog__"

// skillCatalogSource 是目录 item 的 source 字面量。它只在 Timeline 内部使用:
// 目录不参与来源分类渲染, 也没有对应的公开来源常量。
const skillCatalogSource = "catalog"

// PushSkillCatalog 追加技能目录, 相同目录仍存在时不重复写入。
// 目录与正文共用先 Open 后冻结的展示边界。
func (m *Timeline) PushSkillCatalog(id int64, rendered string) bool {
	if m == nil || id <= 0 {
		return false
	}
	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return false
	}
	payload, err := json.Marshal(skillContentPayload{Name: skillCatalogKey, Content: rendered, Source: skillCatalogSource})
	if err != nil {
		return false
	}
	return m.PushPromotable(id, TimelinePromotedKindSkillContent,
		TimelinePromotedTargetSemiDynamic1, skillCatalogKey, TimelinePromotedOperationUpsert, string(payload))
}

// skillMDPathPlaceholder 表示技能主文档 (SKILL.md) 的固定路径位。
const skillMDPathPlaceholder = "SKILL.md"

// skillMainBodyPathSentinel 标记“技能整体视图”(说明 + 文件树 + 阅读窗口) 的
// 对象 Key, 与具体文件路径区分。渲染时不作为路径展示。
//
// 该字面量必须与 aiskillloader.SkillMainBodyPath 保持一致 (两边不能互相
// import, 故各自定义; timeline_skill_content_test.go 断言二者相等)。
const skillMainBodyPathSentinel = "\x00main"

// timelineSkillContentOpenPrompt 渲染尚未冻结 (Open 尾段) 的 Skill 正文。
// 冻结后该 item 不再出现在 Open, 由 renderPromotedSkillContent 提升渲染。
func timelineSkillContentOpenPrompt(op *PromotableTimelineItem) string {
	if op == nil || op.Kind != TimelinePromotedKindSkillContent {
		return ""
	}
	if op.Operation == TimelinePromotedOperationDelete {
		return fmt.Sprintf("[skill_delta]:\n[id: %s]\n[TOMBSTONE] 此技能正文已移出上下文。", op.Key)
	}
	if op.Operation != TimelinePromotedOperationUpsert {
		return fmt.Sprintf("[skill_delta]:\n[id: %s]\n[INVALID] 无效技能正文变更，不改变已有内容。", op.Key)
	}
	var payload skillContentPayload
	if err := json.Unmarshal([]byte(op.Payload), &payload); err != nil {
		return fmt.Sprintf("[skill_delta]:\n[id: %s]\n[INVALID] 无效技能正文载荷。", op.Key)
	}
	body := skillContentBody(payload)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	// Open 段不是缓存前缀, 用可识别的技能段标签包裹, 便于观测与模型区分;
	// 冻结提交后同一正文会改由 renderPromotedSkillContent 的独立技能块渲染。
	nonce := StablePromptNonce("skill-content-open", fmt.Sprint(op.ID))
	return fmt.Sprintf("<|SKILLS_CONTEXT_OPEN_%s|>\n# item=%d [skill_content]\n%s\n<|SKILLS_CONTEXT_OPEN_END_%s|>",
		nonce, op.ID, body, nonce)
}

func skillContentBody(payload skillContentPayload) string {
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = "unknown"
	}
	content := strings.TrimSpace(payload.Content)
	if content == "" {
		return ""
	}
	if name == skillCatalogKey || strings.TrimSpace(payload.Source) == skillCatalogSource {
		// 目录是派生信息, 按目录原文渲染, 不套用技能正文 header。
		return content
	}
	var buf strings.Builder
	path := strings.TrimSpace(payload.Path)
	if path == "" || path == skillMDPathPlaceholder || path == skillMainBodyPathSentinel {
		// 主文档保持与既有 SKILLS_CONTEXT 段落一致的 header 形态。
		fmt.Fprintf(&buf, "=== Skill: %s ===\n", name)
	} else {
		fmt.Fprintf(&buf, "=== Skill: %s (%s) ===\n", name, path)
	}
	switch strings.TrimSpace(payload.Source) {
	case SkillContentSourceForced:
		buf.WriteString("[source: user-forced]\n")
	case SkillContentSourceAuto:
		buf.WriteString("[source: intent-auto-loaded]\n")
	}
	buf.WriteString(content)
	return buf.String()
}

// hasDuplicateSkillContentLocked 只在写入时检查同一技能文件的相同正文是否仍然存在。
// 来源只是加载提示, 不参与正文去重; 已退休正文不阻止重新加载。
func (m *Timeline) hasDuplicateSkillContentLocked(key, body string) bool {
	var incoming skillContentPayload
	if json.Unmarshal([]byte(body), &incoming) != nil {
		return false
	}
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindSkillContent ||
			op.Operation != TimelinePromotedOperationUpsert || op.Key != key {
			continue
		}
		var existing skillContentPayload
		if json.Unmarshal([]byte(op.Payload), &existing) == nil &&
			skillContentKey(existing.Name, existing.Path) == skillContentKey(incoming.Name, incoming.Path) &&
			strings.TrimSpace(existing.Content) == strings.TrimSpace(incoming.Content) {
			return true
		}
	}
	return false
}

// PushSkillContent 追加一条 Skill 正文 item。它不冻结、不提升、不渲染:
// 只有既有冻结/压缩事务提交后, 正文才允许进入半动态段的独立技能块。
// 同一技能文件的相同正文仍存在时返回 false; 不同窗口或内容变化正常追加。
//
// 关键词: PushSkillContent, 先 Open 后冻结提升, 复用选 ID 压缩
func (m *Timeline) PushSkillContent(id int64, name, path, content, source string) bool {
	if m == nil || id <= 0 {
		return false
	}
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if name == "" || content == "" {
		return false
	}
	payload, err := json.Marshal(skillContentPayload{
		Name: name, Path: strings.TrimSpace(path), Content: content, Source: strings.TrimSpace(source),
	})
	if err != nil {
		return false
	}
	return m.PushPromotable(id, TimelinePromotedKindSkillContent,
		TimelinePromotedTargetSemiDynamic1, skillContentKey(name, path), TimelinePromotedOperationUpsert, string(payload))
}

// HasVisibleSkillContent 判断某个技能当前是否还有正文留在上下文里 (Open 或已提
// 升都算)。被显式 unload 退休的正文返回 false, 允许重新读取。
//
// 关键词: HasVisibleSkillContent, 已可见判断, 显式移除后可重读
func (m *Timeline) HasVisibleSkillContent(name string) bool {
	if m == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindSkillContent {
			continue
		}
		var payload skillContentPayload
		if json.Unmarshal([]byte(op.Payload), &payload) != nil {
			continue
		}
		if strings.TrimSpace(payload.Name) == name && strings.TrimSpace(payload.Content) != "" {
			return true
		}
	}
	return false
}

// VisibleSkillContentNames 按 item 顺序返回仍有正文的技能名, 包含 Open 和冻结部分。
// 目录不算已加载技能; 同一技能的多个阅读窗口只计一次。
func (m *Timeline) VisibleSkillContentNames() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := append([]int64(nil), m.idToTimelineItem.Keys()...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var names []string
	seen := make(map[string]bool)
	for _, id := range ids {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindSkillContent ||
			op.Operation != TimelinePromotedOperationUpsert {
			continue
		}
		var payload skillContentPayload
		if json.Unmarshal([]byte(op.Payload), &payload) != nil {
			continue
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" || name == skillCatalogKey || payload.Source == skillCatalogSource ||
			strings.TrimSpace(payload.Content) == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// HasSkillContentHistory 判断该技能是否在本 Timeline 中留下过任何 skill-content
// 记录 (即使正文已被显式移除)。用于区分两种恢复场景:
//   - 有历史: 这是新版本会话, 正文的可见性完全由压缩结果决定, 不能用旧名单重新展开;
//   - 无历史: 是真正的旧会话, 允许一次性导入。
//
// 关键词: HasSkillContentHistory, 恢复边界, 不重新展开已退休正文
func (m *Timeline) HasSkillContentHistory(name string) bool {
	if m == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindSkillContent {
			continue
		}
		var payload skillContentPayload
		if json.Unmarshal([]byte(op.Payload), &payload) != nil {
			continue
		}
		if strings.TrimSpace(payload.Name) == name {
			return true
		}
	}
	return false
}

// DeleteSkillCatalog 退休当前可见的技能目录 item。目录是派生信息, 清空或隐藏
// 目录时不应继续渲染; 该操作不影响技能正文, 也不回滚冻结边界。
func (m *Timeline) DeleteSkillCatalog() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	retired := 0
	for _, id := range append([]int64(nil), m.idToTimelineItem.Keys()...) {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindSkillContent || op.Key != skillCatalogKey {
			continue
		}
		m.retireTimelineItemLocked(id)
		retired++
	}
	if retired == 0 {
		return false
	}
	m.rebuildPromotedStateLocked(m.frozenThroughLocked())
	return true
}

// DeleteSkillContent 退休该技能在 Timeline 中现存的全部正文版本 (journal +
// 已提升态)。删除只改变“下次冻结事务的成员集合”, 不回滚已经提交的冻结边界,
// 因此不会破坏当前可缓存前缀; 已退休的 item 仍保留在 journal 中供审计,
// 且同一技能之后再次读取会重新追加一份新正文。
//
// 关键词: DeleteSkillContent, 退休而不回收 ID, 不触碰冻结边界
func (m *Timeline) DeleteSkillContent(name string) bool {
	if m == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	retired := 0
	for _, existingID := range append([]int64(nil), m.idToTimelineItem.Keys()...) {
		item, ok := m.idToTimelineItem.Get(existingID)
		if !ok || item == nil || item.deleted {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindSkillContent {
			continue
		}
		var payload skillContentPayload
		if json.Unmarshal([]byte(op.Payload), &payload) != nil {
			continue
		}
		if strings.TrimSpace(payload.Name) != name {
			continue
		}
		// 只退休内容, 不回滚冻结边界: 已冻结批次保留原成员集合, 渲染侧
		// 跳过 deleted item, 上游可缓存前缀不会因此被重排。
		m.retireTimelineItemLocked(existingID)
		retired++
	}
	if retired == 0 {
		return false
	}
	m.rebuildPromotedStateLocked(m.frozenThroughLocked())
	log.Infof("delete skill content %q: retired %d timeline item(s)", name, retired)
	return true
}

// appendPromotedSkillContent 将同名技能提升到一条累计正文中。
// 相邻来源 hash 相同则忽略, 不同则拼接正文; 原始 item 始终留在 Timeline 中。
func appendPromotedSkillContent(entries map[string]*PromotedTimelineEntry, item *PromotableTimelineItem) {
	if item.Operation == TimelinePromotedOperationDelete {
		delete(entries, strings.SplitN(item.Key, "\x00", 2)[0])
		return
	}
	if item.Operation != TimelinePromotedOperationUpsert {
		return
	}
	var payload skillContentPayload
	if json.Unmarshal([]byte(item.Payload), &payload) != nil {
		return
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return
	}
	hash := item.PayloadHash
	if hash == "" {
		hash = promotedPayloadHash(item.Payload)
	}
	previous := entries[name]
	if previous != nil && previous.PayloadHash == hash {
		return
	}
	body := skillContentBody(payload)
	if strings.TrimSpace(body) == "" {
		return
	}
	if previous != nil {
		previous.Payload += "\n" + body
		previous.PayloadHash = hash
		return
	}
	entries[name] = &PromotedTimelineEntry{
		Kind: item.Kind, TargetSection: item.TargetSection, Key: name,
		Payload: body, PayloadHash: hash, SourceItemID: item.ID,
	}
}

// renderPromotedSkillContent 渲染 promotedState 中已冻结、仍保留的累计技能正文。
//
// Skill 不再按加载来源分块渲染: 不论用户指定、AI 自动还是普通加载, 都按
// item ID 顺序进入同一个技能块, 加载来源只保留在 payload 里供压缩与观测使用。
//
// 关键词: renderPromotedSkillContent, 冻结后提升, 单一技能块
func renderPromotedSkillContent(state *TimelinePromotedState) string {
	if state == nil {
		return ""
	}
	entries := state.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindSkillContent]
	ordered := make([]*PromotedTimelineEntry, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			ordered = append(ordered, entry)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SourceItemID < ordered[j].SourceItemID })

	var body strings.Builder
	wrote := 0
	for _, entry := range ordered {
		rendered := strings.TrimSpace(entry.Payload)
		if rendered == "" {
			continue
		}
		if wrote > 0 {
			body.WriteString("\n")
		}
		body.WriteString(rendered)
		wrote++
	}
	if wrote == 0 {
		return ""
	}
	nonce := StablePromptNonce("session-skill-content")
	return fmt.Sprintf("<|SKILLS_CONTEXT_%s|>\n== Currently Loaded Skills (from Timeline) ==\n\n%s\n<|SKILLS_CONTEXT_END_%s|>",
		nonce, strings.TrimSpace(body.String()), nonce)
}

// RenderSkillContentForPrompt 返回当前可供模型阅读的、已冻结提升的 Skill 正文。
func (m *Timeline) RenderSkillContentForPrompt() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return renderPromotedSkillContent(m.promotedState)
}

// SkillContentTimelineAdapter 实现 aiskillloader.SkillContentSink: 把 Timeline
// 适配成技能正文的追加/查询/移除入口。ID 由注入的分配器提供, 与既有 Timeline ID
// 分配保持同一来源。
type SkillContentTimelineAdapter struct {
	timeline  *Timeline
	acquireID func() int64
}

func NewSkillContentTimelineAdapter(timeline *Timeline, acquireID func() int64) *SkillContentTimelineAdapter {
	if timeline == nil || acquireID == nil {
		return nil
	}
	return &SkillContentTimelineAdapter{timeline: timeline, acquireID: acquireID}
}

func (a *SkillContentTimelineAdapter) AppendSkillContent(name, path, content, source string) bool {
	if a == nil || a.timeline == nil || a.acquireID == nil {
		return false
	}
	return a.timeline.PushSkillContent(a.acquireID(), name, path, content, source)
}

func (a *SkillContentTimelineAdapter) DeleteSkillCatalog() bool {
	if a == nil || a.timeline == nil {
		return false
	}
	return a.timeline.DeleteSkillCatalog()
}

func (a *SkillContentTimelineAdapter) DeleteSkillContent(name string) bool {
	if a == nil || a.timeline == nil {
		return false
	}
	return a.timeline.DeleteSkillContent(name)
}

func (a *SkillContentTimelineAdapter) HasVisibleSkillContent(name string) bool {
	if a == nil || a.timeline == nil {
		return false
	}
	return a.timeline.HasVisibleSkillContent(name)
}

func (a *SkillContentTimelineAdapter) VisibleSkillContentNames() []string {
	if a == nil || a.timeline == nil {
		return nil
	}
	return a.timeline.VisibleSkillContentNames()
}

// HasSkillContentHistory 供 manager 判断持久会话恢复时是否可以按旧名单展开正文。
func (a *SkillContentTimelineAdapter) HasSkillContentHistory(name string) bool {
	if a == nil || a.timeline == nil {
		return false
	}
	return a.timeline.HasSkillContentHistory(name)
}

func (a *SkillContentTimelineAdapter) RenderSkillContentForPrompt() string {
	if a == nil || a.timeline == nil {
		return ""
	}
	return a.timeline.RenderSkillContentForPrompt()
}

// AppendSkillCatalog 追加“最相关 SKILL 目录”, 仍存在的完全相同目录不会重复写入。
func (a *SkillContentTimelineAdapter) AppendSkillCatalog(rendered string) bool {
	if a == nil || a.timeline == nil {
		return false
	}
	return a.timeline.PushSkillCatalog(a.acquireID(), rendered)
}
