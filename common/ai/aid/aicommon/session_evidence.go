package aicommon

import (
	"fmt"
	"strings"
)

type SessionEvidencePromptBlocks struct {
	Frozen string
	Open   string
}

type EvidenceItemSnapshot struct {
	ID      string
	Content string
}

type SessionEvidenceRenderState struct {
	LastFrozenTimeUnix int64
	LastFrozenRendered string

	FrozenItems map[string]EvidenceItemSnapshot
	FrozenOrder []string
}

func NewSessionEvidenceRenderState() *SessionEvidenceRenderState {
	return &SessionEvidenceRenderState{
		FrozenItems: make(map[string]EvidenceItemSnapshot),
	}
}

func RenderSessionEvidenceFrozenOpen(
	state *SessionEvidenceRenderState,
	store *EvidenceStore,
	frozenTimeUnix int64,
) SessionEvidencePromptBlocks {
	if state == nil {
		state = NewSessionEvidenceRenderState()
	}
	if store == nil {
		store = NewEvidenceStore()
	}
	if frozenTimeUnix < state.LastFrozenTimeUnix {
		resetSessionEvidenceRenderState(state)
	}

	frozen := state.LastFrozenRendered
	if frozenTimeUnix > state.LastFrozenTimeUnix {
		rebuildSessionEvidenceFrozenState(state, store, frozenTimeUnix)
		frozen = state.LastFrozenRendered
	}

	return SessionEvidencePromptBlocks{
		Frozen: frozen,
		Open:   renderSessionEvidenceOpenDelta(store, state),
	}
}

func resetSessionEvidenceRenderState(state *SessionEvidenceRenderState) {
	if state == nil {
		return
	}
	state.LastFrozenTimeUnix = 0
	state.LastFrozenRendered = ""
	state.FrozenItems = make(map[string]EvidenceItemSnapshot)
	state.FrozenOrder = nil
}

func rebuildSessionEvidenceFrozenState(state *SessionEvidenceRenderState, store *EvidenceStore, frozenTimeUnix int64) {
	if state == nil {
		return
	}
	state.LastFrozenTimeUnix = frozenTimeUnix
	state.FrozenItems = make(map[string]EvidenceItemSnapshot)
	state.FrozenOrder = nil

	if store == nil || frozenTimeUnix <= 0 {
		state.LastFrozenRendered = ""
		return
	}

	for _, item := range store.Items {
		id := strings.TrimSpace(item.ID)
		content := strings.TrimSpace(item.Content)
		if id == "" || content == "" {
			continue
		}
		if item.EffectiveUpdatedUnix() >= frozenTimeUnix {
			continue
		}
		state.FrozenItems[id] = EvidenceItemSnapshot{
			ID:      id,
			Content: content,
		}
		state.FrozenOrder = append(state.FrozenOrder, id)
	}
	state.LastFrozenRendered = renderSessionEvidenceFrozenFromState(state)
}

func pruneSessionEvidenceFrozenItem(state *SessionEvidenceRenderState, id string) {
	if state == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if _, ok := state.FrozenItems[id]; !ok {
		return
	}
	delete(state.FrozenItems, id)
	filtered := make([]string, 0, len(state.FrozenOrder))
	for _, frozenID := range state.FrozenOrder {
		if frozenID == id {
			continue
		}
		filtered = append(filtered, frozenID)
	}
	state.FrozenOrder = filtered
	state.LastFrozenRendered = renderSessionEvidenceFrozenFromState(state)
}

func renderSessionEvidenceFrozenFromState(state *SessionEvidenceRenderState) string {
	if state == nil || len(state.FrozenOrder) == 0 {
		return ""
	}
	items := make([]EvidenceItem, 0, len(state.FrozenOrder))
	for _, id := range state.FrozenOrder {
		item, ok := state.FrozenItems[id]
		if !ok {
			continue
		}
		items = append(items, EvidenceItem{ID: item.ID, Content: item.Content})
	}
	return renderEvidenceItems(items)
}

func renderSessionEvidenceOpenDelta(store *EvidenceStore, state *SessionEvidenceRenderState) string {
	if store == nil {
		return ""
	}
	frozenItems := map[string]EvidenceItemSnapshot{}
	frozenOrder := []string(nil)
	if state != nil {
		frozenItems = state.FrozenItems
		frozenOrder = state.FrozenOrder
	}

	liveItems := make(map[string]EvidenceItem)
	liveOrder := make([]string, 0, len(store.Items))
	for _, item := range store.Items {
		id := strings.TrimSpace(item.ID)
		content := strings.TrimSpace(item.Content)
		if id == "" || content == "" {
			continue
		}
		item.ID = id
		item.Content = content
		liveItems[id] = item
		liveOrder = append(liveOrder, id)
	}

	parts := make([]string, 0)
	for _, id := range frozenOrder {
		frozenItem, ok := frozenItems[id]
		if !ok {
			continue
		}
		liveItem, exists := liveItems[id]
		if !exists {
			parts = append(parts, fmt.Sprintf("[id: %s]\n[TOMBSTONE] 此 id 已删除，忽略 frozen 中同 id 内容。", id))
			continue
		}
		if strings.TrimSpace(liveItem.Content) != strings.TrimSpace(frozenItem.Content) {
			parts = append(parts, fmt.Sprintf("[id: %s]\n[OVERRIDE] 此 id 已更新，以 open 为准。\n%s", id, liveItem.Content))
		}
	}
	for _, id := range liveOrder {
		if _, frozen := frozenItems[id]; frozen {
			continue
		}
		liveItem := liveItems[id]
		parts = append(parts, fmt.Sprintf("[id: %s]\n%s", liveItem.ID, liveItem.Content))
	}
	return strings.Join(parts, "\n\n")
}

func renderSessionEvidencePromptBlocks(blocks SessionEvidencePromptBlocks, openNonce string) SessionEvidencePromptBlocks {
	return SessionEvidencePromptBlocks{
		Frozen: RenderSessionEvidenceFrozenPromptBlock(blocks.Frozen),
		Open:   RenderSessionEvidencePromptBlock(openNonce, blocks.Open),
	}
}

func joinSessionEvidencePromptBlocks(blocks SessionEvidencePromptBlocks) string {
	return strings.TrimSpace(strings.Join([]string{blocks.Frozen, blocks.Open}, "\n\n"))
}

func RenderSessionEvidencePromptBlock(nonce string, evidence string) string {
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		return ""
	}
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		nonce = StablePromptNonce("session-evidence-open")
	}
	return fmt.Sprintf(
		"<|SESSION_EVIDENCE_%s|>\n## 已知观测（Evidence）\n\n以下是从真实工具执行中积累的观测结果。它们代表可能性空间中已确认的边界——哪些路径已被排除，哪些路径产生了有价值的信号。\n\n在选择下一步行动前：\n- 检查你的计划是否与已知观测矛盾（不要重复已被排除的路径）\n- 优先沿产生过非基线响应的方向迭代（信息量最大的方向）\n- 如果当前工具的控制能力不足以缩小目标差，考虑换工具或做共轭变换\n\n%s\n<|SESSION_EVIDENCE_END_%s|>",
		nonce, evidence, nonce,
	)
}

func RenderSessionEvidenceFrozenPromptBlock(evidence string) string {
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		return ""
	}
	nonce := StablePromptNonce("session-evidence-frozen")
	return fmt.Sprintf(
		"<|SESSION_EVIDENCE_FROZEN_%s|>\n## 已知观测（Evidence Frozen Snapshot）\n\n%s\n<|SESSION_EVIDENCE_FROZEN_END_%s|>",
		nonce, evidence, nonce,
	)
}
