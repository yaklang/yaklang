package aicommon

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
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

// FormatNextMovementDisplayLine renders a single VerifyNextMovement as the
// chat-friendly one-liner the frontend's "next_movements" stream channel
// expects. The line uses concise English/ASCII markers ([+] add / [DOING] /
// [x] done / [DELETED] / [SKIPPED]) so the same byte sequence is intelligible
// both for the user-facing pseudo-stream and the timeline NEXT_MOVEMENTS
// breadcrumb. Empty input (no id and no content) yields an empty string so
// the caller can choose to skip the line entirely.
//
// 关键词: FormatNextMovementDisplayLine, next_movements 单行渲染,
//
//	verification 与 adjust_todolist 共用 display 行
func FormatNextMovementDisplayLine(movement VerifyNextMovement) string {
	id := strings.TrimSpace(movement.ID)
	content := strings.TrimSpace(movement.Content)
	switch strings.ToLower(strings.TrimSpace(movement.Op)) {
	case "add":
		if id == "" && content == "" {
			return ""
		}
		if id == "" {
			return fmt.Sprintf("- [+]: %s", content)
		}
		if content == "" {
			return fmt.Sprintf("- [+]: [id: %s]", id)
		}
		return fmt.Sprintf("- [+]: [id: %s]: %s", id, content)
	case "doing", "pending":
		if id == "" {
			return ""
		}
		if content == "" {
			return fmt.Sprintf("- [DOING]: [id: %s]", id)
		}
		return fmt.Sprintf("- [DOING]: [id: %s]: %s", id, content)
	case "done":
		if id == "" {
			return ""
		}
		return fmt.Sprintf("- [x]: [id: %s]", id)
	case "delete":
		if id == "" {
			return ""
		}
		if content == "" {
			return fmt.Sprintf("- [DELETED]: [id: %s]", id)
		}
		return fmt.Sprintf("- [DELETED]: [id: %s]: %s", id, content)
	case "skip":
		// 显式跳过, 与 delete 形态对偶, 用于在前端 next_movements stream 中
		// 一眼看出"AI 主动声明这个 TODO 跳过". 与自动翻 SKIPPED 区别开来:
		// 自动翻已废弃, 出现 [SKIPPED] 标签现在都来源于显式 skip op.
		// 关键词: skip op stream 显示, 主动跳过
		if id == "" {
			return ""
		}
		if content == "" {
			return fmt.Sprintf("- [SKIPPED]: [id: %s]", id)
		}
		return fmt.Sprintf("- [SKIPPED]: [id: %s]: %s", id, content)
	default:
		label := strings.ToUpper(strings.TrimSpace(movement.Op))
		if label == "" {
			label = "?"
		}
		if id == "" && content == "" {
			return ""
		}
		if id == "" {
			return fmt.Sprintf("- [%s]: %s", label, content)
		}
		if content == "" {
			return fmt.Sprintf("- [%s]: [id: %s]", label, id)
		}
		return fmt.Sprintf("- [%s]: [id: %s]: %s", label, id, content)
	}
}

// WriteNextMovementsDisplayStream streams a JSON array of next_movements
// from `reader` and writes a newline-separated display rendering to `writer`
// as each element is decoded. It is the shared transformer behind both the
// verification path's "next_movements" stream and the adjust_todolist
// action's "next_movements" StreamHandler, guaranteeing byte-identical
// output whichever channel produced the deltas.
//
// The function returns an error when:
//   - the top-level JSON token is not a `[` (the caller is expected to peek
//     and skip the stream otherwise);
//   - decoding any element fails;
//   - the writer rejects the rendered bytes.
//
// Empty / no-id items are silently dropped — they would render nothing
// useful and only add stray newlines.
//
// 关键词: WriteNextMovementsDisplayStream, next_movements 实时翻译伪流,
//
//	JSON 数组流式解码, verification + adjust_todolist 共用
func WriteNextMovementsDisplayStream(reader io.Reader, writer io.Writer) error {
	decoder := json.NewDecoder(reader)
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '[' {
		return utils.Errorf("next_movements is not a JSON array")
	}

	firstLine := true
	for decoder.More() {
		var movement VerifyNextMovement
		if err := decoder.Decode(&movement); err != nil {
			return err
		}
		line := FormatNextMovementDisplayLine(movement)
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !firstLine {
			if _, err := writer.Write([]byte("\n")); err != nil {
				return err
			}
		}
		firstLine = false
		if _, err := io.WriteString(writer, line); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
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
