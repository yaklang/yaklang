package aiskillloader

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	// ViewWindowMaxBytes is the maximum bytes for a single file view window.
	ViewWindowMaxBytes = 32 * 1024 // 32KB
)

// ViewWindow represents a scrollable view into a file's content.
type ViewWindow struct {
	mu sync.RWMutex

	// SkillName identifies which skill this view belongs to.
	SkillName string

	// FilePath is the path of the file being viewed within the skill filesystem.
	FilePath string

	// Lines holds all lines of the file content.
	Lines []string

	// Offset is the 1-based starting line number for the view.
	Offset int

	// Nonce is the unique identifier for this view window in the prompt.
	Nonce string

	// IsTruncated indicates whether the rendered view was truncated.
	IsTruncated bool
}

// NewViewWindow creates a new view window for a file.
func NewViewWindow(skillName, filePath, content, nonce string) *ViewWindow {
	lines := strings.Split(content, "\n")
	return &ViewWindow{
		SkillName: skillName,
		FilePath:  filePath,
		Lines:     lines,
		Offset:    1,
		Nonce:     nonce,
	}
}

// SetOffset sets the view offset (1-based line number).
// The offset is clamped to valid range [1, totalLines].
func (vw *ViewWindow) SetOffset(offset int) {
	vw.mu.Lock()
	defer vw.mu.Unlock()

	if offset < 1 {
		offset = 1
	}
	if offset > len(vw.Lines) {
		offset = len(vw.Lines)
	}
	vw.Offset = offset
}

// GetOffset returns the current offset.
func (vw *ViewWindow) GetOffset() int {
	vw.mu.RLock()
	defer vw.mu.RUnlock()
	return vw.Offset
}

// IsTruncatedFlag 在窗口锁内读取最近一次渲染的截断标记。
// Render 会在窗口锁内更新该字段, 调用方不能直接读裸字段。
func (vw *ViewWindow) IsTruncatedFlag() bool {
	if vw == nil {
		return false
	}
	vw.mu.RLock()
	defer vw.mu.RUnlock()
	return vw.IsTruncated
}

// TotalLines returns the total number of lines in the file.
func (vw *ViewWindow) TotalLines() int {
	vw.mu.RLock()
	defer vw.mu.RUnlock()
	return len(vw.Lines)
}

// Render renders the view window content into a single string bounded by
// ViewWindowMaxBytes, returning the rendered content and a "was truncated"
// flag.
//
// Two output modes:
//
//  1. Full-view mode (preferred, no line numbers):
//     Triggered when offset == 1 AND the entire file (rendered without any
//     "N | " prefix) fits within ViewWindowMaxBytes. In this mode every line
//     is emitted bare, no leading/trailing "..." ellipsis is added, and
//     truncated is always false.
//
//     Rationale: line numbers exist solely as the input parameter to the
//     change_skill_view_offset action, which the AI only invokes when it
//     needs to scroll a truncated file. If the whole file is already in the
//     view, scrolling is meaningless and line numbers become pure noise that
//     dilutes attention and burns tokens.
//
//  2. Partial-view mode (with line numbers + ellipsis):
//     Used when offset > 1 OR the file is too large to fit unprefixed. Each
//     visible line is rendered as "N | <text>"; a leading "..." marks
//     hidden prefix lines, and a trailing "..." plus truncated=true marks
//     hidden suffix lines. The line numbers here are the contract that lets
//     change_skill_view_offset target a specific line.
//
// 关键词: ViewWindow.Render, 全文模式去行号, isFullView,
//
//	change_skill_view_offset 输入契约, ViewWindowMaxBytes
//
// boundsLocked 计算本次视窗实际覆盖的行。调用方需持有 vw.mu。
// end 小于 start 表示预算内一行都放不下。
func (vw *ViewWindow) boundsLocked() (start, end int, truncated bool) {
	totalLines := len(vw.Lines)
	if totalLines == 0 {
		return 0, 0, false
	}
	offset := vw.Offset
	if offset < 1 {
		offset = 1
	}
	if offset > totalLines {
		offset = totalLines
	}
	headerTag := fmt.Sprintf("<|VIEW_WINDOW_%s|>", vw.Nonce)
	footerTag := fmt.Sprintf("<|VIEW_WINDOW_END_%s|>", vw.Nonce)
	if offset == 1 {
		plainSize := len(headerTag) + 1 + len(footerTag)
		for _, line := range vw.Lines {
			plainSize += len(line) + 1
		}
		if plainSize <= ViewWindowMaxBytes {
			return 1, totalLines, false
		}
	}
	used := len(headerTag) + 1
	if offset > 1 {
		used += len("...\n")
	}
	last := offset - 1
	for i := offset - 1; i < totalLines; i++ {
		line := fmt.Sprintf("%d | %s\n", i+1, vw.Lines[i])
		if used+len(line)+len("...\n")+len(footerTag)+1 > ViewWindowMaxBytes {
			return offset, last, true
		}
		used += len(line)
		last = i + 1
	}
	if last < totalLines {
		truncated = true
	}
	return offset, last, truncated
}

func (vw *ViewWindow) Render() (string, bool) {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	return vw.renderLocked()
}

func (vw *ViewWindow) renderLocked() (string, bool) {
	if len(vw.Lines) == 0 {
		vw.IsTruncated = false
		return "", false
	}
	start, end, truncated := vw.boundsLocked()
	totalLines := len(vw.Lines)
	headerTag := fmt.Sprintf("<|VIEW_WINDOW_%s|>", vw.Nonce)
	footerTag := fmt.Sprintf("<|VIEW_WINDOW_END_%s|>", vw.Nonce)
	fullView := start == 1 && end == totalLines && !truncated
	var buf bytes.Buffer
	buf.WriteString(headerTag)
	buf.WriteString("\n")
	if fullView {
		for _, line := range vw.Lines {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
		buf.WriteString(footerTag)
		vw.IsTruncated = false
		return buf.String(), false
	}
	if start > 1 {
		buf.WriteString("...\n")
	}
	if end >= start {
		for i := start - 1; i < end && i < totalLines; i++ {
			fmt.Fprintf(&buf, "%d | %s\n", i+1, vw.Lines[i])
		}
	}
	if end < totalLines {
		buf.WriteString("...\n")
		truncated = true
	}
	buf.WriteString(footerTag)
	vw.IsTruncated = truncated
	return buf.String(), truncated
}

// Segment 返回本次视窗的正文和行范围。ReachedEnd 表示已经读到文件最后一行。
func (vw *ViewWindow) Segment() (content string, start, end, total int, reachedEnd bool) {
	if vw == nil {
		return "", 0, 0, 0, false
	}
	vw.mu.Lock()
	defer vw.mu.Unlock()
	total = len(vw.Lines)
	start, end, truncated := vw.boundsLocked()
	vw.IsTruncated = truncated
	if start < 1 || end < start || end > total {
		return "", 0, 0, total, false
	}
	content = strings.Join(vw.Lines[start-1:end], "\n")
	return content, start, end, total, end == total
}

// RenderWithInfo renders the view window with a file info header tailored to
// the current view mode:
//
//   - Full-view mode (offset == 1, no truncation): only emit
//     "File: <path> (Skill: <name>)" before the bare content. "Total Lines"
//     and "Current Offset" carry no actionable signal when the entire file is
//     already in view, and would otherwise hint at a scrolling workflow that
//     does not apply.
//   - Partial-view mode (offset > 1 OR content truncated): emit the full
//     header — file label, "Total Lines: N, Current Offset: M", and (when
//     truncated) the change_skill_view_offset scroll hint — so the AI can
//     decide whether to scroll and to which line.
//
// 关键词: ViewWindow.RenderWithInfo, 全文模式精简头部, 部分展示场景
//
//	Total Lines / Current Offset / change_skill_view_offset 提示
func (vw *ViewWindow) RenderWithInfo() string {
	content, truncated := vw.Render()

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("File: %s (Skill: %s)\n", vw.FilePath, vw.SkillName))

	// Detect partial-view mode: we either truncated, or the user scrolled past
	// line 1. Both conditions imply scrolling matters and the metadata header
	// should be present.
	isPartialView := truncated || vw.GetOffset() != 1
	if isPartialView {
		buf.WriteString(fmt.Sprintf("Total Lines: %d, Current Offset: %d\n", vw.TotalLines(), vw.GetOffset()))
		if truncated {
			buf.WriteString(fmt.Sprintf("Note: Content truncated at %dKB limit. Use change_skill_view_offset to scroll.\n", ViewWindowMaxBytes/1024))
		}
	}
	buf.WriteString(content)
	return buf.String()
}

// GenerateNonce creates a deterministic nonce from skill name and file path.
func GenerateNonce(skillName, filePath string) string {
	raw := fmt.Sprintf("%s:%s", skillName, filePath)
	return utils.CalcSha256(raw)[:8]
}
