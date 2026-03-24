package aireact

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

const verificationTodoSnapshotLimit = 10 * 1024

type verificationTodoStatus string

const (
	verificationTodoStatusPending   verificationTodoStatus = "PENDING"
	verificationTodoStatusDone      verificationTodoStatus = "DONE"
	verificationTodoStatusAbandoned verificationTodoStatus = "ABANDONED"
)

type verificationTodoItem struct {
	ID        string
	Content   string
	Status    verificationTodoStatus
	CreatedAt int
	UpdatedAt int
}

func (r *ReAct) AppendVerificationHistory(result *aicommon.VerifySatisfactionResult) {
	if r == nil || result == nil {
		return
	}
	r.verificationHistoryMutex.Lock()
	defer r.verificationHistoryMutex.Unlock()
	cloned := &aicommon.VerifySatisfactionResult{
		Satisfied:          result.Satisfied,
		Reasoning:          result.Reasoning,
		CompletedTaskIndex: result.CompletedTaskIndex,
		NextMovements:      append([]aicommon.VerifyNextMovement(nil), result.NextMovements...),
	}
	r.verificationHistory = append(r.verificationHistory, cloned)
}

func (r *ReAct) RenderVerificationTodoSnapshot() string {
	if r == nil {
		return "- no tracked TODO items"
	}
	r.verificationHistoryMutex.Lock()
	history := append([]*aicommon.VerifySatisfactionResult(nil), r.verificationHistory...)
	r.verificationHistoryMutex.Unlock()
	return renderVerificationTodoSnapshot(history)
}

func renderVerificationTodoSnapshot(history []*aicommon.VerifySatisfactionResult) string {
	items := buildVerificationTodoItems(history)
	if len(items) == 0 {
		return "- no tracked TODO items"
	}

	pending := make([]verificationTodoItem, 0)
	closed := make([]verificationTodoItem, 0)
	for _, item := range items {
		if item.Status == verificationTodoStatusPending {
			pending = append(pending, item)
			continue
		}
		closed = append(closed, item)
	}

	lines := make([]string, 0, len(items)+1)
	for index := len(pending) - 1; index >= 0; index-- {
		lines = append(lines, formatVerificationTodoLine(pending[index]))
	}
	for index := len(closed) - 1; index >= 0; index-- {
		lines = append(lines, formatVerificationTodoLine(closed[index]))
	}

	note := "- NOTE: TODO history exceeded 10KB; older closed items were truncated because this ReAct chain is too long. Prioritize finishing or dropping stale TODOs."
	if len(strings.Join(lines, "\n")) <= verificationTodoSnapshotLimit {
		return strings.Join(lines, "\n")
	}

	truncated := make([]string, 0, len(lines))
	currentBytes := 0
	for _, line := range lines {
		lineBytes := len(line)
		separatorBytes := 0
		if len(truncated) > 0 {
			separatorBytes = 1
		}
		if currentBytes+separatorBytes+lineBytes > verificationTodoSnapshotLimit-len(note)-1 {
			break
		}
		truncated = append(truncated, line)
		currentBytes += separatorBytes + lineBytes
	}
	truncated = append(truncated, note)
	return strings.Join(truncated, "\n")
}

func buildVerificationTodoItems(history []*aicommon.VerifySatisfactionResult) []verificationTodoItem {
	itemsByID := make(map[string]*verificationTodoItem)
	order := make([]string, 0)
	for recordIndex, record := range history {
		if record == nil {
			continue
		}
		for _, movement := range record.NextMovements {
			id := strings.TrimSpace(movement.ID)
			if id == "" {
				continue
			}
			switch movement.Op {
			case "add":
				content := strings.TrimSpace(movement.Content)
				if content == "" {
					continue
				}
				item, exists := itemsByID[id]
				if !exists {
					item = &verificationTodoItem{ID: id, CreatedAt: recordIndex}
					itemsByID[id] = item
					order = append(order, id)
				}
				item.Content = content
				item.Status = verificationTodoStatusPending
				item.UpdatedAt = recordIndex
			case "done":
				item, exists := itemsByID[id]
				if !exists {
					continue
				}
				item.Status = verificationTodoStatusDone
				item.UpdatedAt = recordIndex
			}
		}
		if record.Satisfied {
			for _, id := range order {
				item := itemsByID[id]
				if item == nil || item.Status != verificationTodoStatusPending {
					continue
				}
				item.Status = verificationTodoStatusAbandoned
				item.UpdatedAt = recordIndex
			}
		}
	}

	items := make([]verificationTodoItem, 0, len(order))
	for _, id := range order {
		item := itemsByID[id]
		if item == nil {
			continue
		}
		items = append(items, *item)
	}
	return items
}

func formatVerificationTodoLine(item verificationTodoItem) string {
	statusLabel := "[ ]"
	switch item.Status {
	case verificationTodoStatusDone:
		statusLabel = "[x]"
	case verificationTodoStatusAbandoned:
		statusLabel = "[ABANDONED]"
	}
	content := utils.ShrinkString(strings.TrimSpace(item.Content), 400)
	if content == "" {
		content = "(no content)"
	}
	return fmt.Sprintf("- %s: [id: %s]: %s", statusLabel, item.ID, content)
}
