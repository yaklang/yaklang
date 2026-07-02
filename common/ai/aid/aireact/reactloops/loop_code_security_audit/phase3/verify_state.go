package phase3

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"strings"
	"sync"
)

// VerifyState tracks ordered per-finding verification in Phase 3.
// The AI must conclude findings strictly in scan order; complete_verify is
// allowed only after every finding has been concluded once.
type VerifyState struct {
	mu sync.Mutex

	order     []string
	concluded map[string]bool
}

func newVerifyState(findings []*model.Finding) *VerifyState {
	vs := &VerifyState{
		order:     make([]string, 0, len(findings)),
		concluded: make(map[string]bool),
	}
	for _, f := range findings {
		if f == nil || f.ID == "" {
			continue
		}
		vs.order = append(vs.order, f.ID)
	}
	return vs
}

// SyncFromVerified marks IDs that already have a verified record (resume / retry).
func (v *VerifyState) SyncFromVerified(verified []*model.VerifiedFinding) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, vf := range verified {
		if vf == nil || vf.Finding == nil || vf.Finding.ID == "" {
			continue
		}
		v.concluded[vf.Finding.ID] = true
	}
}

func (v *VerifyState) Total() int {
	if v == nil {
		return 0
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.order)
}

func (v *VerifyState) ConcludedCount() int {
	if v == nil {
		return 0
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	n := 0
	for _, id := range v.order {
		if v.concluded[id] {
			n++
		}
	}
	return n
}

func (v *VerifyState) RemainingCount() int {
	if v == nil {
		return 0
	}
	return v.Total() - v.ConcludedCount()
}

func (v *VerifyState) AllDone() bool {
	if v == nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.order) == 0 {
		return false
	}
	for _, id := range v.order {
		if !v.concluded[id] {
			return false
		}
	}
	return true
}

// CurrentFindingID is the next finding that must receive conclude_finding.
func (v *VerifyState) CurrentFindingID() string {
	if v == nil {
		return ""
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, id := range v.order {
		if !v.concluded[id] {
			return id
		}
	}
	return ""
}

func (v *VerifyState) IsConcluded(findingID string) bool {
	if v == nil || findingID == "" {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.concluded[findingID]
}

func (v *VerifyState) MarkConcluded(findingID string) {
	if v == nil || findingID == "" {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.concluded[findingID] = true
}

// CanConclude reports whether findingID is allowed to be concluded now.
func (v *VerifyState) CanConclude(findingID string) (bool, string) {
	if v == nil {
		return false, "[错误] 内部验证状态未初始化。"
	}
	if findingID == "" {
		return false, "[错误] finding_id 不能为空。"
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	found := false
	for _, id := range v.order {
		if id == findingID {
			found = true
			break
		}
	}
	if !found {
		return false, fmt.Sprintf("[错误] finding_id %q 不在本次审计 finding 列表中。", findingID)
	}
	if v.concluded[findingID] {
		return false, fmt.Sprintf("[错误] %s 已经 conclude_finding 过，禁止重复提交。", findingID)
	}
	for _, id := range v.order {
		if v.concluded[id] {
			continue
		}
		if id != findingID {
			return false, fmt.Sprintf(
				"[错误] 必须按 ID 顺序逐个验证。当前应验证 %s，不能跳过验证 %s。\n"+
					"请先对 %s 调用 conclude_finding，再处理后续 finding。",
				id, findingID, id,
			)
		}
		break
	}
	return true, ""
}

func (v *VerifyState) RemainingIDs() []string {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	var out []string
	for _, id := range v.order {
		if !v.concluded[id] {
			out = append(out, id)
		}
	}
	return out
}

func (v *VerifyState) OrderIDs() []string {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]string, len(v.order))
	copy(out, v.order)
	return out
}

func formatVerifyIDList(ids []string, maxShow int) string {
	if len(ids) == 0 {
		return "  （无）\n"
	}
	var b strings.Builder
	for i, id := range ids {
		if i >= maxShow {
			b.WriteString(fmt.Sprintf("  ... 另有 %d 个未列出\n", len(ids)-maxShow))
			break
		}
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, id))
	}
	return b.String()
}

func formatCompleteVerifyBlockedFeedback(verify *VerifyState) string {
	remaining := verify.RemainingIDs()
	done := verify.ConcludedCount()
	total := verify.Total()
	current := verify.CurrentFindingID()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[错误] 尚有 %d/%d 个 finding 未完成 conclude_finding，禁止调用 complete_verify。\n", total-done, total))
	if current != "" {
		b.WriteString(fmt.Sprintf("当前必须验证：%s\n", current))
	}
	b.WriteString("待验证 finding：\n")
	b.WriteString(formatVerifyIDList(remaining, 30))
	return b.String()
}
