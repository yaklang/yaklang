package ssa

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type Range struct {
	editor     *memedit.MemEditor
	start, end *Position
}

func (p *Range) GetEditor() *memedit.MemEditor {
	return p.editor
}

func (p *Range) GetStart() memedit.PositionIf {
	return p.start
}

func (p *Range) GetEnd() memedit.PositionIf {
	return p.end
}

func NewRange(editor *memedit.MemEditor, start, end *Position) *Range {
	return &Range{
		editor: editor,
		start:  start,
		end:    end,
	}
}

type Position struct {
	Editor *memedit.MemEditor
	Line   int64
	Column int64
}

func (p *Position) GetLine() int {
	return int(p.Line)
}

func (p *Position) GetColumn() int {
	return int(p.Column)
}

func NewPosition(editor *memedit.MemEditor, line, column int64) *Position {
	return &Position{
		Editor: editor,
		Line:   line,
		Column: column,
	}
}

// if ret <  0: p before other
// if ret == 0: p = other
// if ret >  0: p after other
func (p *Range) CompareStart(other *Range) int {
	return p.start.Compare(other.start)
}
func (p *Range) CompareEnd(other *Range) int {
	return p.end.Compare(other.end)
}

func (p *Position) Compare(other *Position) int {
	return int(p.Editor.GetOffsetByPosition(p) - p.Editor.GetOffsetByPosition(other))
}

func (p *Range) GetOffset() int {
	return p.editor.GetOffsetByPosition(p.GetStart())
}

func (p *Range) GetText() string {
	return p.editor.GetTextFromRange(p)
}

func (p *Range) String() string {
	return fmt.Sprintf(
		"%s - %s: %s",
		p.start, p.start, p.GetText(),
	)
}

func (p *Position) String() string {
	return fmt.Sprintf(
		"%d:%d",
		p.Line, p.Column,
	)
}
