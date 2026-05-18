package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/utils"
)

// VerificationTodoSnapshotLimit is the token budget for the rendered TODO snapshot.
//
// 关键词: VerificationTodoSnapshotLimit, TODO token 预算, 10K token
const VerificationTodoSnapshotLimit = 10 * 1024

type VerificationTodoStatus string

const (
	VerificationTodoStatusPending VerificationTodoStatus = "PENDING"
	VerificationTodoStatusDoing   VerificationTodoStatus = "DOING"
	VerificationTodoStatusDone    VerificationTodoStatus = "DONE"
	VerificationTodoStatusDeleted VerificationTodoStatus = "DELETED"
	VerificationTodoStatusSkipped VerificationTodoStatus = "SKIPPED"
)

// VerificationTodoStats summarizes counts of TODO items grouped by status.
type VerificationTodoStats struct {
	Pending int `json:"pending"`
	Doing   int `json:"doing"`
	Done    int `json:"done"`
	Deleted int `json:"deleted"`
	Skipped int `json:"skipped"`
}

// VerificationTodoItem captures a single TODO entry tracked across rounds.
type VerificationTodoItem struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Status    VerificationTodoStatus `json:"status"`
	CreatedAt int                    `json:"created_at"`
	UpdatedAt int                    `json:"updated_at"`
}

// VerificationTodoStore is a session-scoped TODO store maintained incrementally
// by applying each verification round's `next_movements` ops. It replaces the
// previous "rebuild from history" approach so the state survives prompt builds
// without scanning the full history every time.
//
// 关键词: VerificationTodoStore, TODO 增量状态, ApplyOperations, Render,
//
//	RenderMarkdownDelta, prompt 注入, 增量持久化
type VerificationTodoStore struct {
	Items   []*VerificationTodoItem `json:"items"`
	Counter int                     `json:"counter"`
}

// NewVerificationTodoStore returns an empty TODO store.
func NewVerificationTodoStore() *VerificationTodoStore {
	return &VerificationTodoStore{Items: make([]*VerificationTodoItem, 0)}
}

// IsEmpty reports whether the store has no tracked TODO items.
func (s *VerificationTodoStore) IsEmpty() bool {
	if s == nil {
		return true
	}
	return len(s.Items) == 0
}

// Clone returns a deep copy of the store. Useful for delta rendering without
// mutating the live state.
func (s *VerificationTodoStore) Clone() *VerificationTodoStore {
	if s == nil {
		return NewVerificationTodoStore()
	}
	cloned := &VerificationTodoStore{
		Items:   make([]*VerificationTodoItem, 0, len(s.Items)),
		Counter: s.Counter,
	}
	for _, item := range s.Items {
		if item == nil {
			continue
		}
		copyItem := *item
		cloned.Items = append(cloned.Items, &copyItem)
	}
	return cloned
}

// Apply incrementally updates the store with one verification round's
// `next_movements` operations.
//
// 历史: 旧版本在 satisfied == true 时, 会自动把剩余 PENDING/DOING 项翻成
// SKIPPED. 该自动翻转语义已被废弃, 原因如下:
//  1. AI 可能在还有未关闭 TODO 的情况下错误地宣告 user_satisfied=true,
//     自动翻转会掩盖问题, 让 verify gate 直接 Exit 主循环;
//  2. 兜底机制 (ReAct.enforceTodoCompletionBeforeSatisfaction) 需要在
//     Apply 之后观察"是否仍有活跃 TODO", 自动翻转会让兜底永远观察不到.
//
// 新语义: AI 必须通过 next_movements 显式输出 done / delete / skip 来关闭
// 每一个 TODO. satisfied 形参保留是为了接口稳定 (DB 反序列化 + 兼容旧
// 调用方), 但不再触发任何状态变更.
//
// 关键词: Apply 取消自动翻 SKIPPED, 显式关闭, AI 主动 done/delete/skip
func (s *VerificationTodoStore) Apply(satisfied bool, movements []VerifyNextMovement) {
	if s == nil {
		return
	}
	_ = satisfied // 保留形参; 语义见上方注释, 不再触发自动翻转
	s.Counter++
	roundIndex := s.Counter

	for _, movement := range movements {
		id := strings.TrimSpace(movement.ID)
		if id == "" {
			continue
		}
		op := strings.ToLower(strings.TrimSpace(movement.Op))
		switch op {
		case "add":
			content := strings.TrimSpace(movement.Content)
			if content == "" {
				continue
			}
			item := s.findItem(id)
			if item == nil {
				item = &VerificationTodoItem{ID: id, CreatedAt: roundIndex}
				s.Items = append(s.Items, item)
			}
			item.Content = content
			item.Status = VerificationTodoStatusPending
			item.UpdatedAt = roundIndex
		case "doing", "pending":
			item := s.findItem(id)
			if item == nil {
				continue
			}
			if content := strings.TrimSpace(movement.Content); content != "" {
				item.Content = content
			}
			item.Status = VerificationTodoStatusDoing
			item.UpdatedAt = roundIndex
		case "done":
			item := s.findItem(id)
			if item == nil {
				continue
			}
			item.Status = VerificationTodoStatusDone
			item.UpdatedAt = roundIndex
		case "delete":
			item := s.findItem(id)
			if item == nil {
				continue
			}
			if content := strings.TrimSpace(movement.Content); content != "" {
				item.Content = content
			}
			item.Status = VerificationTodoStatusDeleted
			item.UpdatedAt = roundIndex
		case "skip":
			// 显式跳过: AI 主动声明"这个 TODO 暂不做, 但也不算删除".
			// 与 delete 的区别在于语义层面 — delete 表示"不再需要", skip 表
			// 示"本次任务范围内不做". 状态上都是终态, 不再算 active TODO.
			// 关键词: 显式 skip op, 主动跳过, 终态状态
			item := s.findItem(id)
			if item == nil {
				continue
			}
			if content := strings.TrimSpace(movement.Content); content != "" {
				item.Content = content
			}
			item.Status = VerificationTodoStatusSkipped
			item.UpdatedAt = roundIndex
		}
	}
}

func (s *VerificationTodoStore) findItem(id string) *VerificationTodoItem {
	if s == nil {
		return nil
	}
	for _, item := range s.Items {
		if item != nil && item.ID == id {
			return item
		}
	}
	return nil
}

// SnapshotItems returns a deep-copied slice of the current items, safe for
// callers to mutate or serialize.
func (s *VerificationTodoStore) SnapshotItems() []VerificationTodoItem {
	if s == nil {
		return nil
	}
	out := make([]VerificationTodoItem, 0, len(s.Items))
	for _, item := range s.Items {
		if item == nil {
			continue
		}
		out = append(out, *item)
	}
	return out
}

// HasActiveTodos reports whether the store still tracks any PENDING or DOING
// item. This is the primary signal consumed by the Satisfied bottom-line
// override: when the AI declares user_satisfied=true while
// HasActiveTodos() == true, the verification result is rolled back to
// user_satisfied=false so the loop keeps pushing on the unfinished TODOs.
//
// 关键词: HasActiveTodos, Satisfied 兜底信号, 仍有未关闭 TODO 检测
func (s *VerificationTodoStore) HasActiveTodos() bool {
	if s == nil {
		return false
	}
	for _, item := range s.Items {
		if item == nil {
			continue
		}
		if item.Status == VerificationTodoStatusPending || item.Status == VerificationTodoStatusDoing {
			return true
		}
	}
	return false
}

// ActiveTodoItems returns a deep-copied snapshot containing only PENDING /
// DOING items in their original ordering. Used by the Satisfied bottom-line
// override to build a human-readable "remaining TODOs" report for the
// timeline breadcrumb pushed to the AI.
//
// 关键词: ActiveTodoItems, 残留 TODO 快照, Satisfied 兜底 timeline 输入
func (s *VerificationTodoStore) ActiveTodoItems() []VerificationTodoItem {
	if s == nil {
		return nil
	}
	out := make([]VerificationTodoItem, 0)
	for _, item := range s.Items {
		if item == nil {
			continue
		}
		if item.Status != VerificationTodoStatusPending && item.Status != VerificationTodoStatusDoing {
			continue
		}
		out = append(out, *item)
	}
	return out
}

// Stats returns counts grouped by status.
func (s *VerificationTodoStore) Stats() VerificationTodoStats {
	stats := VerificationTodoStats{}
	if s == nil {
		return stats
	}
	for _, item := range s.Items {
		if item == nil {
			continue
		}
		switch item.Status {
		case VerificationTodoStatusPending:
			stats.Pending++
		case VerificationTodoStatusDoing:
			stats.Doing++
		case VerificationTodoStatusDone:
			stats.Done++
		case VerificationTodoStatusDeleted:
			stats.Deleted++
		case VerificationTodoStatusSkipped:
			stats.Skipped++
		}
	}
	return stats
}

// Render returns a plain-text snapshot of TODO items, suitable for the prompt
// TODO block. Active items (doing/pending) are listed first, followed by
// closed items (done/deleted/skipped). Output is capped at
// VerificationTodoSnapshotLimit tokens and truncated when necessary.
func (s *VerificationTodoStore) Render() string {
	if s == nil || len(s.Items) == 0 {
		return "- no tracked TODO items"
	}

	pending := make([]VerificationTodoItem, 0)
	doing := make([]VerificationTodoItem, 0)
	closed := make([]VerificationTodoItem, 0)
	for _, item := range s.Items {
		if item == nil {
			continue
		}
		switch item.Status {
		case VerificationTodoStatusPending:
			pending = append(pending, *item)
		case VerificationTodoStatusDoing:
			doing = append(doing, *item)
		default:
			closed = append(closed, *item)
		}
	}

	// 倒序输出每组, 让"最近更新"先被 LLM 看到
	// 关键词: 倒序展示, 最近优先
	lines := make([]string, 0, len(s.Items)+1)
	for index := len(doing) - 1; index >= 0; index-- {
		lines = append(lines, FormatVerificationTodoLine(doing[index]))
	}
	for index := len(pending) - 1; index >= 0; index-- {
		lines = append(lines, FormatVerificationTodoLine(pending[index]))
	}
	for index := len(closed) - 1; index >= 0; index-- {
		lines = append(lines, FormatVerificationTodoLine(closed[index]))
	}

	note := "- NOTE: TODO history exceeded 10K tokens; older closed items were truncated because this ReAct chain is too long. Prioritize finishing or dropping stale TODOs."
	if ytoken.CalcTokenCount(strings.Join(lines, "\n")) <= VerificationTodoSnapshotLimit {
		return strings.Join(lines, "\n")
	}

	truncated := make([]string, 0, len(lines))
	currentTokens := 0
	for _, line := range lines {
		lineTokens := ytoken.CalcTokenCount(line)
		separatorTokens := 0
		if len(truncated) > 0 {
			separatorTokens = 1
		}
		if currentTokens+separatorTokens+lineTokens > VerificationTodoSnapshotLimit-ytoken.CalcTokenCount(note)-1 {
			break
		}
		truncated = append(truncated, line)
		currentTokens += separatorTokens + lineTokens
	}
	truncated = append(truncated, note)
	return strings.Join(truncated, "\n")
}

// RenderMarkdownDelta renders the markdown snapshot for emitting to the
// frontend after a verification round. It applies `movements` (and
// `satisfied`) on a clone of the current state, marking items that became new
// / doing / done / deleted / skipped during this very round.
//
// 与 plain Render 不同, 该输出携带 (new)/(doing)/(done)/(deleted)/(skipped)
// 这些 marker, 让前端 markdown 通道能高亮本轮变化.
//
// 关键词: RenderMarkdownDelta, markdown 增量标记, frontend stream
func (s *VerificationTodoStore) RenderMarkdownDelta(satisfied bool, movements []VerifyNextMovement) string {
	previous := s
	if previous == nil {
		previous = NewVerificationTodoStore()
	}
	previousIDs := make(map[string]struct{}, len(previous.Items))
	for _, item := range previous.Items {
		if item != nil {
			previousIDs[item.ID] = struct{}{}
		}
	}

	cloned := previous.Clone()
	cloned.Apply(satisfied, movements)
	if len(cloned.Items) == 0 {
		return ""
	}

	currentNewIDs := make(map[string]struct{})
	currentDoneIDs := make(map[string]struct{})
	currentSkippedIDs := make(map[string]struct{})
	for _, movement := range movements {
		id := strings.TrimSpace(movement.ID)
		if id == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(movement.Op)) {
		case "add":
			if _, exists := previousIDs[id]; !exists {
				currentNewIDs[id] = struct{}{}
			}
		case "done":
			currentDoneIDs[id] = struct{}{}
		case "skip":
			// 本轮显式 skip 的 TODO, 在 markdown delta 中需要打上 (skipped)
			// marker, 与 done / deleted 形成对偶的关闭信号.
			// 关键词: RenderMarkdownDelta skip marker, 显式跳过高亮
			currentSkippedIDs[id] = struct{}{}
		}
	}

	oldPending := make([]string, 0)
	doingItems := make([]string, 0)
	newPending := make([]string, 0)
	oldDone := make([]string, 0)
	currentDone := make([]string, 0)
	deleted := make([]string, 0)
	oldSkipped := make([]string, 0)
	currentSkipped := make([]string, 0)

	for _, item := range cloned.Items {
		if item == nil {
			continue
		}
		switch item.Status {
		case VerificationTodoStatusPending:
			if _, isNew := currentNewIDs[item.ID]; isNew {
				newPending = append(newPending, FormatVerificationTodoMarkdownLine(*item, "new"))
			} else {
				oldPending = append(oldPending, FormatVerificationTodoMarkdownLine(*item, ""))
			}
		case VerificationTodoStatusDoing:
			doingItems = append(doingItems, FormatVerificationTodoMarkdownLine(*item, "doing"))
		case VerificationTodoStatusDone:
			if _, isDone := currentDoneIDs[item.ID]; isDone {
				currentDone = append(currentDone, FormatVerificationTodoMarkdownLine(*item, "done"))
			} else {
				oldDone = append(oldDone, FormatVerificationTodoMarkdownLine(*item, ""))
			}
		case VerificationTodoStatusDeleted:
			deleted = append(deleted, FormatVerificationTodoMarkdownLine(*item, "deleted"))
		case VerificationTodoStatusSkipped:
			if _, isSkipped := currentSkippedIDs[item.ID]; isSkipped {
				currentSkipped = append(currentSkipped, FormatVerificationTodoMarkdownLine(*item, "skipped"))
			} else {
				// 历史轮次已经被 skip 的 TODO 不应该每轮都带 (skipped)
				// marker, 否则前端会把它当成"本轮新发生的变化"反复闪一下.
				// 关键词: 历史 SKIPPED 不再高亮, 仅本轮 skip 才带 marker
				oldSkipped = append(oldSkipped, FormatVerificationTodoMarkdownLine(*item, ""))
			}
		}
	}

	lines := append([]string{}, doingItems...)
	lines = append(lines, oldPending...)
	lines = append(lines, newPending...)
	lines = append(lines, oldDone...)
	lines = append(lines, currentDone...)
	lines = append(lines, deleted...)
	lines = append(lines, oldSkipped...)
	lines = append(lines, currentSkipped...)

	note := "- [x] (truncated) TODO snapshot exceeded 10K tokens; older items were omitted to keep the view stable."
	if ytoken.CalcTokenCount(strings.Join(lines, "\n")) <= VerificationTodoSnapshotLimit {
		return strings.Join(lines, "\n")
	}

	truncated := make([]string, 0, len(lines))
	currentTokens := 0
	for _, line := range lines {
		lineTokens := ytoken.CalcTokenCount(line)
		separatorTokens := 0
		if len(truncated) > 0 {
			separatorTokens = 1
		}
		if currentTokens+separatorTokens+lineTokens > VerificationTodoSnapshotLimit-ytoken.CalcTokenCount(note)-1 {
			break
		}
		truncated = append(truncated, line)
		currentTokens += separatorTokens + lineTokens
	}
	truncated = append(truncated, note)
	return strings.Join(truncated, "\n")
}

// Marshal returns a JSON-encoded representation of the store, suitable for
// persistence in SessionPromptState.
func (s *VerificationTodoStore) Marshal() string {
	if s == nil {
		return `{"items":[],"counter":0}`
	}
	data, err := json.Marshal(s)
	if err != nil {
		return `{"items":[],"counter":0}`
	}
	return string(data)
}

// UnmarshalVerificationTodoStore decodes a JSON string produced by Marshal,
// falling back to an empty store when the payload is empty or malformed.
func UnmarshalVerificationTodoStore(data string) *VerificationTodoStore {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return NewVerificationTodoStore()
	}
	store := &VerificationTodoStore{}
	if err := json.Unmarshal([]byte(trimmed), store); err == nil {
		if store.Items == nil {
			store.Items = make([]*VerificationTodoItem, 0)
		}
		return store
	}
	return NewVerificationTodoStore()
}

// FormatVerificationTodoLine renders a single item line for the prompt TODO
// block. The format is intentionally compatible with the previous
// `formatVerificationTodoLine` output so existing tests / prompt examples keep
// passing.
//
// 关键词: FormatVerificationTodoLine, [DOING] [DELETED] [SKIPPED] [x] [ ]
func FormatVerificationTodoLine(item VerificationTodoItem) string {
	statusLabel := "[ ]"
	switch item.Status {
	case VerificationTodoStatusDoing:
		statusLabel = "[DOING]"
	case VerificationTodoStatusDone:
		statusLabel = "[x]"
	case VerificationTodoStatusDeleted:
		statusLabel = "[DELETED]"
	case VerificationTodoStatusSkipped:
		statusLabel = "[SKIPPED]"
	}
	content := utils.ShrinkString(strings.TrimSpace(item.Content), 400)
	if content == "" {
		content = "(no content)"
	}
	return fmt.Sprintf("- %s: [id: %s]: %s", statusLabel, item.ID, content)
}

// FormatVerificationTodoMarkdownLine renders a single item line for the
// markdown stream emitted at the end of a verification round. The output
// format matches the previous `formatVerificationTodoMarkdownLine` (delta
// markers like (new) / (doing) / (done) / (deleted) / (skipped)).
func FormatVerificationTodoMarkdownLine(item VerificationTodoItem, marker string) string {
	statusLabel := "[ ]"
	switch item.Status {
	case VerificationTodoStatusDone, VerificationTodoStatusDeleted, VerificationTodoStatusSkipped:
		statusLabel = "[x]"
	}
	content := SanitizeVerificationTodoMarkdownContent(item.Content)
	if item.Status == VerificationTodoStatusDone || item.Status == VerificationTodoStatusDeleted {
		content = "~~" + content + "~~"
	}
	if marker == "" && item.Status == VerificationTodoStatusDeleted {
		marker = "deleted"
	}
	if marker == "" {
		return fmt.Sprintf("- %s %s", statusLabel, content)
	}
	return fmt.Sprintf("- %s (%s) %s", statusLabel, marker, content)
}

// SanitizeVerificationTodoMarkdownContent collapses line breaks / tabs / other
// whitespace into single spaces so a single TODO item never injects extra
// markdown bullets into the emitted stream.
//
// 关键词: SanitizeVerificationTodoMarkdownContent, 防 markdown 注入,
//
//	UnicodeLineSep U+2028, ParagraphSep U+2029 替换
func SanitizeVerificationTodoMarkdownContent(content string) string {
	replacer := strings.NewReplacer(
		"\r", " ",
		"\n", " ",
		"\t", " ",
		"\u2028", " ",
		"\u2029", " ",
	)
	content = replacer.Replace(content)
	content = strings.Join(strings.Fields(content), " ")
	content = utils.ShrinkString(strings.TrimSpace(content), 400)
	if content == "" {
		return "(no content)"
	}
	return content
}
