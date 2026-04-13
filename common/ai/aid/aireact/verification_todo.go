package aireact

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/utils"
)

// verificationTodoSnapshotLimit is measured in tokens (not bytes).
const verificationTodoSnapshotLimit = 10 * 1024

type verificationTodoStatus string

const (
	verificationTodoStatusPending verificationTodoStatus = "PENDING"
	verificationTodoStatusDoing   verificationTodoStatus = "DOING"
	verificationTodoStatusDone    verificationTodoStatus = "DONE"
	verificationTodoStatusDeleted verificationTodoStatus = "DELETED"
	verificationTodoStatusSkipped verificationTodoStatus = "SKIPPED"
)

type verificationTodoStats struct {
	Pending int
	Doing   int
	Done    int
	Deleted int
	Skipped int
}

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
		Satisfied:             result.Satisfied,
		Reasoning:             result.Reasoning,
		CompletedTaskIndex:    result.CompletedTaskIndex,
		NextMovements:         append([]aicommon.VerifyNextMovement(nil), result.NextMovements...),
		CoveredTargets:        append([]string(nil), result.CoveredTargets...),
		MissingTargets:        append([]string(nil), result.MissingTargets...),
		SatisfiedRequirements: append([]string(nil), result.SatisfiedRequirements...),
		MissingRequirements:   append([]string(nil), result.MissingRequirements...),
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

func (r *ReAct) RenderVerificationTodoMarkdownSnapshot(current *aicommon.VerifySatisfactionResult) string {
	if r == nil {
		return ""
	}
	r.verificationHistoryMutex.Lock()
	history := append([]*aicommon.VerifySatisfactionResult(nil), r.verificationHistory...)
	r.verificationHistoryMutex.Unlock()
	return renderVerificationTodoMarkdownSnapshot(history, current)
}

func (r *ReAct) RenderVerificationOutputFilesMarkdown(outputFiles []string) string {
	if r == nil {
		return ""
	}
	return renderVerificationOutputFilesMarkdown(outputFiles)
}

func (r *ReAct) RenderVerificationCoverageMarkdown(result *aicommon.VerifySatisfactionResult) string {
	if r == nil || result == nil {
		return ""
	}
	return renderVerificationCoverageMarkdown(result)
}

func renderVerificationTodoSnapshot(history []*aicommon.VerifySatisfactionResult) string {
	items, _ := buildVerificationTodoItemsAndStats(history)
	if len(items) == 0 {
		return "- no tracked TODO items"
	}

	pending := make([]verificationTodoItem, 0)
	doing := make([]verificationTodoItem, 0)
	closed := make([]verificationTodoItem, 0)
	for _, item := range items {
		if item.Status == verificationTodoStatusPending {
			pending = append(pending, item)
			continue
		}
		if item.Status == verificationTodoStatusDoing {
			doing = append(doing, item)
			continue
		}
		closed = append(closed, item)
	}

	lines := make([]string, 0, len(items)+1)
	for index := len(doing) - 1; index >= 0; index-- {
		lines = append(lines, formatVerificationTodoLine(doing[index]))
	}
	for index := len(pending) - 1; index >= 0; index-- {
		lines = append(lines, formatVerificationTodoLine(pending[index]))
	}
	for index := len(closed) - 1; index >= 0; index-- {
		lines = append(lines, formatVerificationTodoLine(closed[index]))
	}

	note := "- NOTE: TODO history exceeded 10K tokens; older closed items were truncated because this ReAct chain is too long. Prioritize finishing or dropping stale TODOs."
	if ytoken.CalcTokenCount(strings.Join(lines, "\n")) <= verificationTodoSnapshotLimit {
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
		if currentTokens+separatorTokens+lineTokens > verificationTodoSnapshotLimit-ytoken.CalcTokenCount(note)-1 {
			break
		}
		truncated = append(truncated, line)
		currentTokens += separatorTokens + lineTokens
	}
	truncated = append(truncated, note)
	return strings.Join(truncated, "\n")
}

func renderVerificationCoverageMarkdown(result *aicommon.VerifySatisfactionResult) string {
	if result == nil {
		return ""
	}
	coveredTargets := aicommon.FormatVerificationCoverageSummary(result.CoveredTargets, nil, nil, nil)
	_ = coveredTargets
	sections := make([]string, 0, 4)
	if len(result.CoveredTargets) > 0 {
		sections = append(sections, "### Covered Targets\n- "+strings.Join(result.CoveredTargets, "\n- "))
	}
	if len(result.MissingTargets) > 0 {
		sections = append(sections, "### Missing Targets\n- "+strings.Join(result.MissingTargets, "\n- "))
	}
	if len(result.SatisfiedRequirements) > 0 {
		sections = append(sections, "### Satisfied Requirements\n- "+strings.Join(result.SatisfiedRequirements, "\n- "))
	}
	if len(result.MissingRequirements) > 0 {
		sections = append(sections, "### Missing Requirements\n- "+strings.Join(result.MissingRequirements, "\n- "))
	}
	return strings.Join(sections, "\n\n")
}

func renderVerificationTodoMarkdownSnapshot(history []*aicommon.VerifySatisfactionResult, current *aicommon.VerifySatisfactionResult) string {
	previousItems, _ := buildVerificationTodoItemsAndStats(history)
	previousIDs := make(map[string]struct{}, len(previousItems))
	for _, item := range previousItems {
		previousIDs[item.ID] = struct{}{}
	}

	if current != nil {
		history = append(append([]*aicommon.VerifySatisfactionResult(nil), history...), current)
	}
	items, _ := buildVerificationTodoItemsAndStats(history)
	if len(items) == 0 {
		return ""
	}

	currentNewIDs := make(map[string]struct{})
	currentDoneIDs := make(map[string]struct{})
	if current != nil {
		for _, movement := range current.NextMovements {
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
			}
		}
	}

	oldPending := make([]string, 0)
	doingItems := make([]string, 0)
	newPending := make([]string, 0)
	oldDone := make([]string, 0)
	currentDone := make([]string, 0)
	deleted := make([]string, 0)
	skipped := make([]string, 0)

	for _, item := range items {
		switch item.Status {
		case verificationTodoStatusPending:
			if _, isNew := currentNewIDs[item.ID]; isNew {
				newPending = append(newPending, formatVerificationTodoMarkdownLine(item, "new"))
			} else {
				oldPending = append(oldPending, formatVerificationTodoMarkdownLine(item, ""))
			}
		case verificationTodoStatusDoing:
			doingItems = append(doingItems, formatVerificationTodoMarkdownLine(item, "doing"))
		case verificationTodoStatusDone:
			if _, isDone := currentDoneIDs[item.ID]; isDone {
				currentDone = append(currentDone, formatVerificationTodoMarkdownLine(item, "done"))
			} else {
				oldDone = append(oldDone, formatVerificationTodoMarkdownLine(item, ""))
			}
		case verificationTodoStatusDeleted:
			deleted = append(deleted, formatVerificationTodoMarkdownLine(item, "deleted"))
		case verificationTodoStatusSkipped:
			skipped = append(skipped, formatVerificationTodoMarkdownLine(item, "skipped"))
		}
	}

	lines := append([]string{}, doingItems...)
	lines = append(lines, oldPending...)
	lines = append(lines, newPending...)
	lines = append(lines, oldDone...)
	lines = append(lines, currentDone...)
	lines = append(lines, deleted...)
	lines = append(lines, skipped...)

	note := "- [x] (truncated) TODO snapshot exceeded 10K tokens; older items were omitted to keep the view stable."
	if ytoken.CalcTokenCount(strings.Join(lines, "\n")) <= verificationTodoSnapshotLimit {
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
		if currentTokens+separatorTokens+lineTokens > verificationTodoSnapshotLimit-ytoken.CalcTokenCount(note)-1 {
			break
		}
		truncated = append(truncated, line)
		currentTokens += separatorTokens + lineTokens
	}
	truncated = append(truncated, note)
	return strings.Join(truncated, "\n")
}

func buildVerificationTodoItems(history []*aicommon.VerifySatisfactionResult) []verificationTodoItem {
	items, _ := buildVerificationTodoItemsAndStats(history)
	return items
}

func buildVerificationTodoItemsAndStats(history []*aicommon.VerifySatisfactionResult) ([]verificationTodoItem, verificationTodoStats) {
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
			switch strings.ToLower(strings.TrimSpace(movement.Op)) {
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
			case "doing", "pending":
				item, exists := itemsByID[id]
				if !exists {
					continue
				}
				if content := strings.TrimSpace(movement.Content); content != "" {
					item.Content = content
				}
				item.Status = verificationTodoStatusDoing
				item.UpdatedAt = recordIndex
			case "done":
				item, exists := itemsByID[id]
				if !exists {
					continue
				}
				item.Status = verificationTodoStatusDone
				item.UpdatedAt = recordIndex
			case "delete":
				item, exists := itemsByID[id]
				if !exists {
					continue
				}
				if content := strings.TrimSpace(movement.Content); content != "" {
					item.Content = content
				}
				item.Status = verificationTodoStatusDeleted
				item.UpdatedAt = recordIndex
			}
		}
		if record.Satisfied {
			for _, id := range order {
				item := itemsByID[id]
				if item == nil || (item.Status != verificationTodoStatusPending && item.Status != verificationTodoStatusDoing) {
					continue
				}
				item.Status = verificationTodoStatusSkipped
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
	return items, calculateVerificationTodoStats(items)
}

func calculateVerificationTodoStats(items []verificationTodoItem) verificationTodoStats {
	stats := verificationTodoStats{}
	for _, item := range items {
		switch item.Status {
		case verificationTodoStatusPending:
			stats.Pending++
		case verificationTodoStatusDoing:
			stats.Doing++
		case verificationTodoStatusDone:
			stats.Done++
		case verificationTodoStatusDeleted:
			stats.Deleted++
		case verificationTodoStatusSkipped:
			stats.Skipped++
		}
	}
	return stats
}

func formatVerificationTodoLine(item verificationTodoItem) string {
	statusLabel := "[ ]"
	switch item.Status {
	case verificationTodoStatusDoing:
		statusLabel = "[DOING]"
	case verificationTodoStatusDone:
		statusLabel = "[x]"
	case verificationTodoStatusDeleted:
		statusLabel = "[DELETED]"
	case verificationTodoStatusSkipped:
		statusLabel = "[SKIPPED]"
	}
	content := utils.ShrinkString(strings.TrimSpace(item.Content), 400)
	if content == "" {
		content = "(no content)"
	}
	return fmt.Sprintf("- %s: [id: %s]: %s", statusLabel, item.ID, content)
}

func formatVerificationTodoMarkdownLine(item verificationTodoItem, marker string) string {
	statusLabel := "[ ]"
	switch item.Status {
	case verificationTodoStatusDone, verificationTodoStatusDeleted, verificationTodoStatusSkipped:
		statusLabel = "[x]"
	}
	content := sanitizeVerificationTodoMarkdownContent(item.Content)
	if item.Status == verificationTodoStatusDone || item.Status == verificationTodoStatusDeleted {
		content = "~~" + content + "~~"
	}
	if marker == "" && item.Status == verificationTodoStatusDeleted {
		marker = "deleted"
	}
	if marker == "" {
		return fmt.Sprintf("- %s %s", statusLabel, content)
	}
	return fmt.Sprintf("- %s (%s) %s", statusLabel, marker, content)
}

func sanitizeVerificationTodoMarkdownContent(content string) string {
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

func renderVerificationOutputFilesMarkdown(outputFiles []string) string {
	normalized := normalizeVerificationOutputFiles(outputFiles)
	if len(normalized) == 0 {
		return ""
	}

	lines := make([]string, 0, len(normalized)+3)
	for _, filePath := range normalized {
		lines = append(lines, fmt.Sprintf("- %s", filePath))
	}
	return strings.Join(lines, "\n")
}

func normalizeVerificationOutputFiles(outputFiles []string) []string {
	if len(outputFiles) == 0 {
		return nil
	}

	result := make([]string, 0, len(outputFiles))
	seen := make(map[string]struct{}, len(outputFiles))
	for _, filePath := range outputFiles {
		normalizedPath := sanitizeVerificationOutputFilePath(filePath)
		if normalizedPath == "" {
			continue
		}
		if _, exists := seen[normalizedPath]; exists {
			continue
		}
		seen[normalizedPath] = struct{}{}
		result = append(result, normalizedPath)
	}
	return result
}

func sanitizeVerificationOutputFilePath(filePath string) string {
	cleaned := strings.TrimSpace(filePath)
	if cleaned == "" {
		return ""
	}
	cleaned = strings.NewReplacer("\r", "", "\n", "", "\t", " ").Replace(cleaned)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	base := filepath.Base(cleaned)
	if strings.HasPrefix(base, "ai_bash_script_") && strings.HasSuffix(base, ".sh") {
		return ""
	}
	return cleaned
}
