package aicommon

import (
	"fmt"
	"strings"
)

// NormalizeVerifyNextMovements parses the `next_movements` field of an
// Action (the same shape used by `verify-satisfaction` results) into a slice
// of canonical VerifyNextMovement records. The normalization rules are:
//
//   - Each item must carry a non-empty `op` and `id`; items missing either
//     are silently dropped.
//   - `op` is lower-cased before matching; the historical alias `pending` is
//     folded into `doing` so downstream consumers see only the canonical
//     status set (add / doing / done / delete / skip).
//   - When the structured array is absent but a legacy string fallback is
//     present at the top-level `next_movements` field, a single-item slice
//     is synthesised with `op=add`, `id=legacy_next_movements`, content
//     copied from the legacy text. This keeps prompts that still emit the
//     old freeform string compatible with the new TODO store apply path.
//
// 关键词: NormalizeVerifyNextMovements, next_movements 解析, pending->doing,
//
//	legacy string 兼容, 全局 TODO 通道
func NormalizeVerifyNextMovements(action *Action) []VerifyNextMovement {
	if action == nil {
		return nil
	}
	nextMovementsRaw := action.GetInvokeParamsArray("next_movements")
	nextMovements := make([]VerifyNextMovement, 0, len(nextMovementsRaw))
	for _, movement := range nextMovementsRaw {
		if movement == nil {
			continue
		}
		op := strings.ToLower(strings.TrimSpace(movement.GetString("op")))
		if op == "pending" {
			op = "doing"
		}
		id := strings.TrimSpace(movement.GetString("id"))
		content := strings.TrimSpace(movement.GetString("content"))
		if op == "" || id == "" {
			continue
		}
		nextMovements = append(nextMovements, VerifyNextMovement{
			Op:      op,
			Content: content,
			ID:      id,
		})
	}
	if len(nextMovements) > 0 {
		return nextMovements
	}

	legacy := strings.TrimSpace(action.GetString("next_movements"))
	if legacy == "" {
		return nil
	}
	return []VerifyNextMovement{{
		Op:      "add",
		ID:      "legacy_next_movements",
		Content: legacy,
	}}
}

// FormatNextMovementsBreadcrumb renders a compact one-line-per-op summary of
// the supplied next_movements slice. The output is the same breadcrumb the
// verification path writes to the timeline under the "NEXT_MOVEMENTS" key,
// preserving the chronological signal consumers (UI / test / log analysis)
// rely on to answer "when was the TODO updated?".
//
// Empty input yields an empty string, allowing callers to skip the timeline
// write entirely when the round produced no movement.
//
// 关键词: FormatNextMovementsBreadcrumb, delta-only timeline 事件, TODO 时间戳信号
func FormatNextMovementsBreadcrumb(movements []VerifyNextMovement) string {
	if len(movements) == 0 {
		return ""
	}
	lines := make([]string, 0, len(movements))
	for _, m := range movements {
		op := strings.ToUpper(strings.TrimSpace(m.Op))
		if op == "" {
			op = "ADD"
		}
		id := strings.TrimSpace(m.ID)
		content := strings.TrimSpace(m.Content)
		switch {
		case id != "" && content != "":
			lines = append(lines, fmt.Sprintf("%s[%s]: %s", op, id, content))
		case id != "":
			lines = append(lines, fmt.Sprintf("%s[%s]", op, id))
		case content != "":
			lines = append(lines, fmt.Sprintf("%s: %s", op, content))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}
