package aicommon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"

	"github.com/yaklang/yaklang/common/log"
)

// skillContentPayload 区分技能的两类内容:
// metadata 是说明、关联技能和文件树; content 是一个文件的一段行范围。
// Range.ReachedEnd 为 true 时, 这一段已经到达文件末尾。
//
// Source 只作为来源提示, 不参与去重, 也不决定渲染分块。
type skillContentRange struct {
	StartLine  int  `json:"start_line,omitempty"`
	EndLine    int  `json:"end_line,omitempty"`
	Total      int  `json:"total,omitempty"`
	ReachedEnd bool `json:"reached_end,omitempty"`
}

type skillContentPayload struct {
	Name    string             `json:"name"`
	Kind    string             `json:"kind,omitempty"`
	Path    string             `json:"path,omitempty"`
	Range   *skillContentRange `json:"range,omitempty"`
	Content string             `json:"content"`
	Source  string             `json:"source,omitempty"`
}

const (
	skillContentKindMetadata = "metadata"
	skillContentKindContent  = "content"
)

// Skill 正文来源取值 (与 aiskillloader.SkillContentSource* 保持一致)。
// 此处不 import aiskillloader 以避免无谓耦合, 仅做字面量校验。
const (
	SkillContentSourceAuto   = "auto"
	SkillContentSourceForced = "forced"
	SkillContentSourceManual = "manual"
)

func isSkillCatalogPayload(payload skillContentPayload) bool {
	return strings.TrimSpace(payload.Name) == skillCatalogKey || strings.TrimSpace(payload.Source) == skillCatalogSource
}

func normalizeSkillContentPayload(payload *skillContentPayload) {
	if payload == nil {
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Kind = strings.TrimSpace(payload.Kind)
	payload.Path = strings.TrimSpace(payload.Path)
	payload.Source = strings.TrimSpace(payload.Source)
	payload.Content = strings.TrimSpace(payload.Content)
	if isSkillCatalogPayload(*payload) {
		payload.Kind = ""
		payload.Path = ""
		payload.Range = nil
		return
	}
	if payload.Kind == skillContentKindMetadata {
		payload.Path = ""
		payload.Range = nil
		return
	}
	payload.Kind = skillContentKindContent
	if payload.Path == "" {
		payload.Path = skillMDPathPlaceholder
	}
	if payload.Range == nil {
		return
	}
	if payload.Range.StartLine < 0 {
		payload.Range.StartLine = 0
	}
	if payload.Range.EndLine < 0 {
		payload.Range.EndLine = 0
	}
	if payload.Range.Total < 0 {
		payload.Range.Total = 0
	}
	if payload.Range.StartLine == 0 && payload.Range.EndLine == 0 && payload.Range.Total == 0 {
		payload.Range = nil
		return
	}
	payload.Range.ReachedEnd = payload.Range.Total > 0 && payload.Range.EndLine == payload.Range.Total
}

// skillPartIdentity 是一条技能内容的身份: 元信息按技能名, 文件内容按路径和行范围。
// 相同文本出现在不同范围时不是重复内容。
func skillPartIdentity(payload skillContentPayload) string {
	normalizeSkillContentPayload(&payload)
	if isSkillCatalogPayload(payload) {
		return skillCatalogKey
	}
	if payload.Kind == skillContentKindMetadata {
		return payload.Name + "\x00metadata"
	}
	start, end := 0, 0
	if payload.Range != nil {
		start, end = payload.Range.StartLine, payload.Range.EndLine
	}
	return fmt.Sprintf("%s\x00content\x00%s\x00%d\x00%d", payload.Name, payload.Path, start, end)
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
	body := skillOpenText(payload)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	// Open 段不是缓存前缀, 用可识别的技能段标签包裹, 便于观测与模型区分;
	// 冻结提交后同一正文会改由 renderPromotedSkillContent 的独立技能块渲染。
	nonce := StablePromptNonce("skill-open", fmt.Sprint(op.ID))
	return fmt.Sprintf("<|SKILLS_CONTEXT_OPEN_%s|>\n# item=%d [skill]\n%s\n<|SKILLS_CONTEXT_OPEN_END_%s|>",
		nonce, op.ID, body, nonce)
}

func skillSourceLine(source string) string {
	switch strings.TrimSpace(source) {
	case SkillContentSourceForced:
		return "[source: user-forced]"
	case SkillContentSourceAuto:
		return "[source: intent-auto-loaded]"
	default:
		return ""
	}
}

func skillRangeLabel(payload skillContentPayload) string {
	path := payload.Path
	if path == "" {
		path = skillMDPathPlaceholder
	}
	if payload.Range == nil || payload.Range.EndLine <= 0 {
		return path
	}
	label := fmt.Sprintf("%s:%d-%d/%d", path, payload.Range.StartLine, payload.Range.EndLine, payload.Range.Total)
	if payload.Range.ReachedEnd {
		label += " END"
	}
	return label
}

func skillOpenText(payload skillContentPayload) string {
	normalizeSkillContentPayload(&payload)
	content := payload.Content
	if content == "" {
		return ""
	}
	if isSkillCatalogPayload(payload) {
		return content
	}
	name := payload.Name
	if name == "" {
		name = "unknown"
	}
	var buf strings.Builder
	if payload.Kind == skillContentKindMetadata {
		fmt.Fprintf(&buf, "[skill %s | metadata]\n", name)
		if source := skillSourceLine(payload.Source); source != "" {
			buf.WriteString(source)
			buf.WriteString("\n")
		}
	} else {
		fmt.Fprintf(&buf, "[skill %s | %s]\n", name, skillRangeLabel(payload))
	}
	buf.WriteString(content)
	return buf.String()
}

func skillPromotedFragment(payload skillContentPayload) string {
	normalizeSkillContentPayload(&payload)
	content := payload.Content
	if content == "" {
		return ""
	}
	if isSkillCatalogPayload(payload) {
		return content
	}
	var buf strings.Builder
	if payload.Kind == skillContentKindMetadata {
		buf.WriteString("[metadata]\n")
		if source := skillSourceLine(payload.Source); source != "" {
			buf.WriteString(source)
			buf.WriteString("\n")
		}
	} else {
		fmt.Fprintf(&buf, "[%s]\n", skillRangeLabel(payload))
	}
	buf.WriteString(content)
	return buf.String()
}

// hasDuplicateSkillContentLocked 按技能部分身份和正文检查完全重复。
// 来源不参与比较; 已退休正文不阻止重新加载; 不同行范围即使文本相同也不是重复。
func (m *Timeline) hasDuplicateSkillContentLocked(identity string, body string) bool {
	var incoming skillContentPayload
	if json.Unmarshal([]byte(body), &incoming) != nil {
		return false
	}
	normalizeSkillContentPayload(&incoming)
	incomingContent := incoming.Content
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindSkillContent ||
			op.Operation != TimelinePromotedOperationUpsert {
			continue
		}
		var existing skillContentPayload
		if json.Unmarshal([]byte(op.Payload), &existing) != nil {
			continue
		}
		if skillPartIdentity(existing) == identity && strings.TrimSpace(existing.Content) == incomingContent {
			return true
		}
	}
	return false
}

func (m *Timeline) pushSkillContentPayload(id int64, payload skillContentPayload) bool {
	if m == nil || id <= 0 {
		return false
	}
	normalizeSkillContentPayload(&payload)
	if payload.Name == "" || payload.Content == "" {
		return false
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	return m.PushPromotable(id, TimelinePromotedKindSkillContent,
		TimelinePromotedTargetSemiDynamic1, skillPartIdentity(payload), TimelinePromotedOperationUpsert, string(raw))
}

// PushSkillMetadata 追加技能说明、关联技能和文件树。
func (m *Timeline) PushSkillMetadata(id int64, name, content, source string) bool {
	return m.pushSkillContentPayload(id, skillContentPayload{
		Name: name, Kind: skillContentKindMetadata, Content: content, Source: source,
	})
}

// PushSkillFileContent 追加一个文件范围。EndLine 等于 Total 时标记为文件已翻完。
func (m *Timeline) PushSkillFileContent(id int64, name, path, content, source string, startLine, endLine, total int) bool {
	return m.pushSkillContentPayload(id, skillContentPayload{
		Name: name, Kind: skillContentKindContent, Path: path, Content: content, Source: source,
		Range: &skillContentRange{StartLine: startLine, EndLine: endLine, Total: total},
	})
}

// SkillContextPart 是仍在上下文中的一个技能部分。同一路径和范围只出现一次。
type SkillContextPart struct {
	Name       string
	Kind       string
	Path       string
	StartLine  int
	EndLine    int
	Total      int
	ReachedEnd bool
	// TokenCount 累加该部分仍在 Open 或已提升的正文，不计渲染标签与目录。
	TokenCount int
	// Content 是该部分仍在 Open 或已提升的正文。同一路径和范围存在多个不同
	// 版本时，在可见结果中按 Timeline 顺序保留。
	Content string
}

// VisibleSkillParts 返回目标技能当前仍可见的 metadata 和 content。
// name 为空时返回全部技能。目录和已退休 item 不计入。
// TokenCount 按可见正文逐条计量；同一范围的不同内容全部计入，冻结聚合丢弃的
// 相邻同 hash 条目不计入。该查询不改变 journal、冻结状态或 promotedState。
func (m *Timeline) VisibleSkillParts(name string) []SkillContextPart {
	if m == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := append([]int64(nil), m.idToTimelineItem.Keys()...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var parts []SkillContextPart
	index := map[string]int{}
	frozenHashes := map[string]string{}
	through := m.frozenThroughLocked()
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
		normalizeSkillContentPayload(&payload)
		if isSkillCatalogPayload(payload) || payload.Content == "" {
			continue
		}
		if name != "" && payload.Name != name {
			continue
		}
		identity := skillPartIdentity(payload)
		position, found := index[identity]
		if !found {
			position = len(parts)
			index[identity] = position
			part := SkillContextPart{Name: payload.Name, Kind: payload.Kind, Path: payload.Path}
			if payload.Range != nil {
				part.StartLine = payload.Range.StartLine
				part.EndLine = payload.Range.EndLine
				part.Total = payload.Range.Total
				part.ReachedEnd = payload.Range.ReachedEnd
			}
			parts = append(parts, part)
		}
		if id <= through {
			// 与 appendPromotedSkillContent 一致：已冻结的相邻同 hash 只展示一次；
			// Open 条目尚未聚合，不能提前按 hash 把它们从计量中移除。
			hash := op.PayloadHash
			if hash == "" {
				hash = promotedPayloadHash(op.Payload)
			}
			if frozenHashes[identity] == hash {
				continue
			}
			frozenHashes[identity] = hash
		}
		parts[position].TokenCount += MeasureTokens(payload.Content)
		if parts[position].Content != "" {
			parts[position].Content += "\n"
		}
		parts[position].Content += payload.Content
	}
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].Name != parts[j].Name {
			return parts[i].Name < parts[j].Name
		}
		if parts[i].Kind != parts[j].Kind {
			return parts[i].Kind == skillContentKindMetadata
		}
		if parts[i].Path != parts[j].Path {
			return parts[i].Path < parts[j].Path
		}
		if parts[i].StartLine != parts[j].StartLine {
			return parts[i].StartLine < parts[j].StartLine
		}
		return parts[i].EndLine < parts[j].EndLine
	})
	return parts
}

// HasSkillContentHistory 判断该技能是否在本 Timeline 中留下过任何 skill
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

// appendPromotedSkillContent 按“技能名 + 路径 + 行范围”提升。
// 同一身份 hash 相同则忽略, 不同则拼接; 不同范围分别保留。
func appendPromotedSkillContent(entries map[string]*PromotedTimelineEntry, item *PromotableTimelineItem) {
	if item == nil {
		return
	}
	if item.Operation == TimelinePromotedOperationDelete {
		var payload skillContentPayload
		if json.Unmarshal([]byte(item.Payload), &payload) == nil && strings.TrimSpace(payload.Name) != "" {
			delete(entries, skillPartIdentity(payload))
			return
		}
		delete(entries, item.Key)
		return
	}
	if item.Operation != TimelinePromotedOperationUpsert {
		return
	}
	var payload skillContentPayload
	if json.Unmarshal([]byte(item.Payload), &payload) != nil {
		return
	}
	normalizeSkillContentPayload(&payload)
	if payload.Name == "" && !isSkillCatalogPayload(payload) {
		return
	}
	key := skillPartIdentity(payload)
	hash := item.PayloadHash
	if hash == "" {
		hash = promotedPayloadHash(item.Payload)
	}
	previous := entries[key]
	if previous != nil && previous.PayloadHash == hash {
		return
	}
	body := skillPromotedFragment(payload)
	if strings.TrimSpace(body) == "" {
		return
	}
	if previous != nil {
		previous.Payload += "\n" + body
		previous.PayloadHash = hash
		return
	}
	entries[key] = &PromotedTimelineEntry{
		Kind: item.Kind, TargetSection: item.TargetSection, Key: key,
		Payload: body, PayloadHash: hash, SourceItemID: item.ID,
	}
}

type skillPromotedGroup struct {
	name    string
	catalog bool
	firstID int64
	parts   []*PromotedTimelineEntry
}

func skillPromotedGroupKey(entry *PromotedTimelineEntry) (group string, catalog bool, kind, path string, start, end int) {
	if entry == nil {
		return "", false, "", "", 0, 0
	}
	if entry.Key == skillCatalogKey {
		return skillCatalogKey, true, "", "", 0, 0
	}
	fields := strings.Split(entry.Key, "\x00")
	if len(fields) >= 2 && fields[1] == skillContentKindMetadata {
		return fields[0], false, skillContentKindMetadata, "", 0, 0
	}
	if len(fields) == 5 && fields[1] == skillContentKindContent {
		start, _ = strconv.Atoi(fields[3])
		end, _ = strconv.Atoi(fields[4])
		return fields[0], false, skillContentKindContent, fields[2], start, end
	}
	if len(fields) == 2 {
		return fields[0], false, skillContentKindContent, fields[1], 0, 0
	}
	return entry.Key, false, skillContentKindContent, "", 0, 0
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
		if entry != nil && strings.TrimSpace(entry.Payload) != "" {
			ordered = append(ordered, entry)
		}
	}
	if len(ordered) == 0 {
		return ""
	}
	groups := make([]*skillPromotedGroup, 0)
	index := map[string]int{}
	for _, entry := range ordered {
		groupName, catalog, _, _, _, _ := skillPromotedGroupKey(entry)
		key := groupName
		if catalog {
			key = skillCatalogKey + ":" + fmt.Sprint(entry.SourceItemID)
		}
		position, found := index[key]
		if !found {
			position = len(groups)
			index[key] = position
			groups = append(groups, &skillPromotedGroup{name: groupName, catalog: catalog, firstID: entry.SourceItemID})
		}
		group := groups[position]
		if entry.SourceItemID < group.firstID {
			group.firstID = entry.SourceItemID
		}
		group.parts = append(group.parts, entry)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].firstID < groups[j].firstID })
	var body strings.Builder
	wrote := 0
	for _, group := range groups {
		sort.Slice(group.parts, func(i, j int) bool {
			leftName, _, leftKind, leftPath, leftStart, leftEnd := skillPromotedGroupKey(group.parts[i])
			rightName, _, rightKind, rightPath, rightStart, rightEnd := skillPromotedGroupKey(group.parts[j])
			_ = leftName
			_ = rightName
			if leftKind != rightKind {
				return leftKind == skillContentKindMetadata
			}
			if leftPath != rightPath {
				return leftPath < rightPath
			}
			if leftStart != rightStart {
				return leftStart < rightStart
			}
			if leftEnd != rightEnd {
				return leftEnd < rightEnd
			}
			return group.parts[i].SourceItemID < group.parts[j].SourceItemID
		})
		var section strings.Builder
		if group.catalog {
			for _, part := range group.parts {
				if section.Len() > 0 {
					section.WriteString("\n")
				}
				section.WriteString(strings.TrimSpace(part.Payload))
			}
		} else {
			fmt.Fprintf(&section, "=== Skill: %s ===\n", group.name)
			for index, part := range group.parts {
				if index > 0 {
					section.WriteString("\n")
				}
				section.WriteString(strings.TrimSpace(part.Payload))
			}
		}
		rendered := strings.TrimSpace(section.String())
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
	nonce := StablePromptNonce("session-skill")
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

func (a *SkillContentTimelineAdapter) AppendSkillMetadata(name, content, source string) bool {
	if a == nil || a.timeline == nil || a.acquireID == nil {
		return false
	}
	return a.timeline.PushSkillMetadata(a.acquireID(), name, content, source)
}

func (a *SkillContentTimelineAdapter) AppendSkillFileContent(name, path, content, source string, startLine, endLine, total int) bool {
	if a == nil || a.timeline == nil || a.acquireID == nil {
		return false
	}
	return a.timeline.PushSkillFileContent(a.acquireID(), name, path, content, source, startLine, endLine, total)
}

func (a *SkillContentTimelineAdapter) VisibleSkillParts(name string) []aiskillloader.SkillContextPart {
	if a == nil || a.timeline == nil {
		return nil
	}
	parts := a.timeline.VisibleSkillParts(name)
	out := make([]aiskillloader.SkillContextPart, 0, len(parts))
	for _, part := range parts {
		out = append(out, aiskillloader.SkillContextPart{
			Name: part.Name, Kind: part.Kind, Path: part.Path,
			StartLine: part.StartLine, EndLine: part.EndLine, Total: part.Total, ReachedEnd: part.ReachedEnd,
			TokenCount: part.TokenCount, Content: part.Content,
		})
	}
	return out
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
