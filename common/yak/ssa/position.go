package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"

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

func (r *Range) Len() int {
	start, end := r.GetOffsetRange()
	return end - start + 1
}

func (r *Range) Add(end *Range) {
	r.end = end.end
	r.text = r.editor.GetTextFromOffset(r.GetOffset(), r.GetEndOffset())
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

func (p *Range) GetTextContext(n int) string {
	result, err := p.editor.GetContextAroundRange(p.GetStart(), p.GetEnd(), n)
	if err != nil {
		log.Warnf("editor.GetContextAroundRange(start, end, %v) failed: %v", n, err)
		return ""
	}
	return result
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
