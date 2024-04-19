package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

type Range struct {
	text       string
	editor     *memedit.MemEditor
	start, end *Position
}

func (r *Range) GetEditor() *memedit.MemEditor {
	return r.editor
}

func (r *Range) GetEndOffset() int {
	return r.editor.GetOffsetByPosition(r.end)
}

func (r *Range) GetOffsetRange() (int, int) {
	return r.editor.GetOffsetByPosition(r.start), r.editor.GetOffsetByPosition(r.end)
}

func (r *Range) GetStart() memedit.PositionIf {
	return r.start
}

func (r *Range) GetEnd() memedit.PositionIf {
	return r.end
}

func NewRange(editor *memedit.MemEditor, startIf, endIf memedit.PositionIf) *Range {
	start, ok := startIf.(*Position)
	if !ok {
		start = NewPosition(int64(startIf.GetLine()), int64(startIf.GetColumn()))
	}
	end, ok := endIf.(*Position)
	if !ok {
		end = NewPosition(int64(endIf.GetLine()), int64(endIf.GetColumn()))
	}

	start.Editor = editor
	end.Editor = editor
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

func NewPosition(line, column int64) *Position {
	return &Position{
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
	if p.text != "" {
		return p.text
	}
	p.text = p.editor.GetTextFromRange(p)
	return p.text
}

func (p *Range) GetWordText() string {
	return p.editor.GetWordTextFromRange(p)
}

func (p *Range) String() string {
	return fmt.Sprintf(
		"%s - %s: %s",
		p.start, p.end, p.GetText(),
	)
}

func (p *Position) String() string {
	return fmt.Sprintf(
		"%d:%d",
		p.Line, p.Column,
	)
}
