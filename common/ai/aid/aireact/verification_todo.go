package aireact

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// Type / constant aliases keep the old aireact-local symbol surface
// intact so in-package tests can keep referencing them without churn.
// Real logic lives in aicommon.VerificationTodoStore.
//
// 关键词: aireact <-> aicommon TODO 兼容层, 类型别名, 字符串别名
type verificationTodoStatus = aicommon.VerificationTodoStatus

const (
	verificationTodoStatusPending = aicommon.VerificationTodoStatusPending
	verificationTodoStatusDoing   = aicommon.VerificationTodoStatusDoing
	verificationTodoStatusDone    = aicommon.VerificationTodoStatusDone
	verificationTodoStatusDeleted = aicommon.VerificationTodoStatusDeleted
	verificationTodoStatusSkipped = aicommon.VerificationTodoStatusSkipped
)

type verificationTodoStats = aicommon.VerificationTodoStats
type verificationTodoItem = aicommon.VerificationTodoItem

// verificationTodoSnapshotLimit is measured in tokens (not bytes). Re-exported
// here for backward-compatible test references.
const verificationTodoSnapshotLimit = aicommon.VerificationTodoSnapshotLimit

// AppendVerificationHistory commits one verification round's
// next_movements (and the round's satisfied flag) into the shared
// SessionPromptState TODO store. The TODO list is then visible to the loop
// prompt (timeline-open section) on every subsequent iteration, not only
// inside the next Verify call.
//
// 关键词: AppendVerificationHistory, SessionPromptState 写入,
//
//	Loop prompt TODO 可见性, 全局 TODO
func (r *ReAct) AppendVerificationHistory(result *aicommon.VerifySatisfactionResult) {
	if r == nil || result == nil {
		return
	}
	if r.config == nil {
		return
	}
	r.config.ApplyVerificationTodoOps(result.Satisfied, result.NextMovements)
}

// RenderVerificationTodoSnapshot returns the plain-text TODO snapshot built
// from the shared SessionPromptState store. When the store is empty the
// caller-friendly placeholder "- no tracked TODO items" is returned (the
// caller can choose to suppress empty blocks itself).
func (r *ReAct) RenderVerificationTodoSnapshot() string {
	if r == nil || r.config == nil {
		return "- no tracked TODO items"
	}
	rendered := r.config.GetVerificationTodoRendered()
	if rendered == "" {
		return "- no tracked TODO items"
	}
	return rendered
}

// RenderVerificationTodoMarkdownSnapshot returns the markdown snapshot used by
// the verification markdown stream (with delta markers like (new) / (done)).
// `current` is the not-yet-committed verification result; the function computes
// the snapshot AS IF `current` were applied, leaving the underlying state
// untouched.
func (r *ReAct) RenderVerificationTodoMarkdownSnapshot(current *aicommon.VerifySatisfactionResult) string {
	if r == nil || r.config == nil {
		return ""
	}
	if current == nil {
		// preserve old behaviour: when nothing new is supplied, still surface
		// the current full snapshot via the markdown formatter.
		return r.config.GetVerificationTodoMarkdownDelta(false, nil)
	}
	return r.config.GetVerificationTodoMarkdownDelta(current.Satisfied, current.NextMovements)
}

func (r *ReAct) RenderVerificationOutputFilesMarkdown(outputFiles []string) string {
	if r == nil {
		return ""
	}
	return renderVerificationOutputFilesMarkdown(outputFiles)
}

// renderVerificationTodoSnapshot is kept as a package-local shim so existing
// aireact tests (verification_todo_test.go) can keep operating on a
// VerifySatisfactionResult history slice. It rebuilds a fresh
// VerificationTodoStore by applying each history entry sequentially, then
// renders.
//
// 关键词: renderVerificationTodoSnapshot 兼容层, history -> store
func renderVerificationTodoSnapshot(history []*aicommon.VerifySatisfactionResult) string {
	store := buildVerificationTodoStoreFromHistory(history)
	if store.IsEmpty() {
		return "- no tracked TODO items"
	}
	return store.Render()
}

func renderVerificationTodoMarkdownSnapshot(history []*aicommon.VerifySatisfactionResult, current *aicommon.VerifySatisfactionResult) string {
	store := buildVerificationTodoStoreFromHistory(history)
	if current == nil {
		return store.RenderMarkdownDelta(false, nil)
	}
	return store.RenderMarkdownDelta(current.Satisfied, current.NextMovements)
}

func buildVerificationTodoStoreFromHistory(history []*aicommon.VerifySatisfactionResult) *aicommon.VerificationTodoStore {
	store := aicommon.NewVerificationTodoStore()
	for _, record := range history {
		if record == nil {
			continue
		}
		store.Apply(record.Satisfied, record.NextMovements)
	}
	return store
}

// buildVerificationTodoItems / buildVerificationTodoItemsAndStats remain as
// package-local helpers for back-compat with verification_todo_test.go.
func buildVerificationTodoItems(history []*aicommon.VerifySatisfactionResult) []aicommon.VerificationTodoItem {
	store := buildVerificationTodoStoreFromHistory(history)
	return store.SnapshotItems()
}

func buildVerificationTodoItemsAndStats(history []*aicommon.VerifySatisfactionResult) ([]aicommon.VerificationTodoItem, aicommon.VerificationTodoStats) {
	store := buildVerificationTodoStoreFromHistory(history)
	return store.SnapshotItems(), store.Stats()
}

// Package-local pass-through helpers kept for legacy tests. New code should
// use the aicommon.Format* / aicommon.Sanitize* functions directly.

func formatVerificationTodoLine(item aicommon.VerificationTodoItem) string {
	return aicommon.FormatVerificationTodoLine(item)
}

func formatVerificationTodoMarkdownLine(item aicommon.VerificationTodoItem, marker string) string {
	return aicommon.FormatVerificationTodoMarkdownLine(item, marker)
}

func sanitizeVerificationTodoMarkdownContent(content string) string {
	return aicommon.SanitizeVerificationTodoMarkdownContent(content)
}

// renderVerificationOutputFilesMarkdown renders the per-task delivery file
// listing emitted at verification time. The logic is intentionally local to
// aireact because it has no equivalent in the SessionPromptState store yet.
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
