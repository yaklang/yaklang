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
	ViewWindowMaxBytes = 15 * 1024 // 15KB
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

// TotalLines returns the total number of lines in the file.
func (vw *ViewWindow) TotalLines() int {
	vw.mu.RLock()
	defer vw.mu.RUnlock()
	return len(vw.Lines)
}

// Render renders the view window content with line numbers.
// The output is limited to ViewWindowMaxBytes.
// Returns the rendered content and whether it was truncated.
func (vw *ViewWindow) Render() (string, bool) {
	vw.mu.Lock()
	defer vw.mu.Unlock()

	if len(vw.Lines) == 0 {
		return "", false
	}

	var buf bytes.Buffer
	totalLines := len(vw.Lines)
	offset := vw.Offset
	if offset < 1 {
		offset = 1
	}
	if offset > totalLines {
		offset = totalLines
	}

	// Write header tag
	buf.WriteString(fmt.Sprintf("<|VIEW_WINDOW_%s|>\n", vw.Nonce))

	// Show ellipsis if not starting from line 1
	if offset > 1 {
		buf.WriteString("...\n")
	}

	truncated := false
	lastRenderedLine := offset - 1

	for i := offset - 1; i < totalLines; i++ {
		lineNum := i + 1
		line := fmt.Sprintf("%d | %s\n", lineNum, vw.Lines[i])

		// Check if adding this line would exceed the limit
		endTag := fmt.Sprintf("<|VIEW_WINDOW_END_%s|>", vw.Nonce)
		projectedSize := buf.Len() + len(line) + len("...\n") + len(endTag) + 1
		if projectedSize > ViewWindowMaxBytes {
			truncated = true
			break
		}

		buf.WriteString(line)
		lastRenderedLine = lineNum
	}

	// Show ellipsis if not ending at the last line
	if lastRenderedLine < totalLines {
		buf.WriteString("...\n")
		truncated = true
	}

	// Write footer tag
	buf.WriteString(fmt.Sprintf("<|VIEW_WINDOW_END_%s|>", vw.Nonce))

	vw.IsTruncated = truncated
	return buf.String(), truncated
}

// RenderWithInfo renders the view window with file info header.
func (vw *ViewWindow) RenderWithInfo() string {
	content, truncated := vw.Render()

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("File: %s (Skill: %s)\n", vw.FilePath, vw.SkillName))
	buf.WriteString(fmt.Sprintf("Total Lines: %d, Current Offset: %d\n", vw.TotalLines(), vw.GetOffset()))
	if truncated {
		buf.WriteString(fmt.Sprintf("Note: Content truncated at %dKB limit. Use change_skill_view_offset to scroll.\n", ViewWindowMaxBytes/1024))
	}
	buf.WriteString(content)
	return buf.String()
}

// GenerateNonce creates a deterministic nonce from skill name and file path.
func GenerateNonce(skillName, filePath string) string {
	raw := fmt.Sprintf("%s:%s", skillName, filePath)
	return utils.CalcSha256(raw)[:8]
}
