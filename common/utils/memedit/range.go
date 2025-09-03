package memedit

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
)

type Position struct {
	line   int
	column int
}

func NewPosition(line, column int) *Position {
	return &Position{line: line, column: column}
}

func (p *Position) GetLine() int {
	if p == nil {
		return 0
	}
	return p.line
}

func (p *Position) GetColumn() int {
	if p == nil {
		return 0
	}
	return p.column
}

func (p *Position) String() string {
	if p == nil {
		return "0:0"
	}
	return fmt.Sprintf("%d:%d", p.line, p.column)
}

func (p *Position) SetLine(line int) {
	if p == nil {
		return
	}
	p.line = line
}

func (p *Position) SetColumn(column int) {
	if p == nil {
		return
	}
	p.column = column
}

type Range struct {
	start       *Position
	startOffset int
	end         *Position
	endOffset   int
	editor      *MemEditor
	text        string
}

func NewRange(p1, p2 *Position) *Range {
	if p1 == nil || p2 == nil {
		log.Warn("NewRange called with nil position(s)")
		return nil
	}
	p1line := p1.GetLine()
	p2line := p2.GetLine()
	p1col := p1.GetColumn()
	p2col := p2.GetColumn()
	if p1line < p2line || (p1line == p2line && p1col < p2col) {
		return &Range{start: p1, end: p2}
	}
	return &Range{start: p2, end: p1}
}

func (r *Range) SetEditor(editor *MemEditor) {
	if r == nil {
		return
	}
	r.editor = editor
	if editor != nil {
		r.startOffset = editor.GetOffsetByPosition(r.start)
		r.endOffset = editor.GetOffsetByPosition(r.end)
	}
}

func (r *Range) GetEditor() *MemEditor {
	if r == nil {
		return nil
	}
	return r.editor
}

func (r *Range) GetStart() *Position {
	if r == nil {
		return nil
	}
	return r.start
}

func (r *Range) GetStartOffset() int {
	if r == nil {
		return 0
	}
	return r.startOffset
}

func (r *Range) GetEnd() *Position {
	if r == nil {
		return nil
	}
	return r.end
}

func (r *Range) GetEndOffset() int {
	if r == nil {
		return 0
	}
	return r.endOffset
}

func (p *Range) GetTextContext(n int) string {
	if p == nil || p.editor == nil {
		log.Warn("range or range.editor is nil")
		return ""
	}
	result, err := p.editor.GetContextAroundRange(p.GetStart(), p.GetEnd(), n)
	if err != nil {
		log.Warnf("editor.GetContextAroundRange(start, end, %v) failed: %v", n, err)
		return ""
	}
	return result
}

func (p *Range) GetText() string {
	if p == nil {
		return ""
	}
	if p.text != "" {
		return p.text
	}
	if p.editor == nil {
		log.Warn("range.editor is nil")
		return ""
	}
	p.text = p.editor.GetTextFromRange(p)
	return p.text
}

func (r *Range) Len() int {
	if r == nil {
		return 0
	}
	return r.endOffset - r.startOffset + 1
}

func (r *Range) Add(end *Range) {
	if r == nil || end == nil {
		log.Warn("range or end range is nil")
		return
	}
	if r.editor == nil {
		log.Warn("range.editor is nil")
		return
	}
	r.end = end.GetEnd()
	r.text = r.editor.GetTextFromOffset(r.startOffset, r.endOffset)
}

func (p *Range) String() string {
	if p == nil || p.start == nil || p.end == nil {
		return "nil range"
	}
	return fmt.Sprintf(
		"%s - %s: %s",
		p.start, p.end, p.GetText(),
	)
}

func (p *Range) GetTextContextWithPrompt(n int, msg ...string) string {
	if p == nil || p.editor == nil {
		log.Warn("range or range.editor is nil")
		return ""
	}
	return p.editor.GetTextContextWithPrompt(p, n, msg...)
}

func (p *Range) GetWordText() string {
	if p == nil || p.editor == nil {
		log.Warn("range or range.editor is nil")
		return ""
	}
	return p.editor.GetWordTextFromRange(p)
}
