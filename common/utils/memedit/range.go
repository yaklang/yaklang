package memedit

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
)

type RangeIf interface {
	// position
	GetStart() PositionIf
	GetStartOffset() int
	GetEnd() PositionIf
	GetEndOffset() int
	// editor
	GetEditor() *MemEditor
	// text
	GetText() string
	GetWordText() string
	GetTextContext(int) string
	GetTextContextWithPrompt(n int, msg ...string) string
	Len() int
	// extern
	Add(end RangeIf)

	// to string
	String() string
}

type PositionIf interface {
	GetLine() int
	GetColumn() int
	// to string
	String() string
}

type EditablePositionIf interface {
	SetLine(int)
	SetColumn(int)
}

var (
	_ RangeIf            = (*codeRange)(nil)
	_ PositionIf         = (*position)(nil)
	_ EditablePositionIf = (*position)(nil)
)

type position struct {
	line   int
	column int
}

func NewPosition(line, column int) *position {
	return &position{line: line, column: column}
}

func (p *position) GetLine() int {
	return p.line
}

func (p *position) GetColumn() int {
	return p.column
}

func (p *position) String() string {
	return fmt.Sprintf("%d:%d", p.line, p.column)
}

func (p *position) SetLine(line int) {
	p.line = line
}

func (p *position) SetColumn(column int) {
	p.column = column
}

type codeRange struct {
	start       PositionIf
	startOffset int
	end         PositionIf
	endOffset   int
	editor      *MemEditor
	text        string
}

func NewRange(p1, p2 PositionIf) *codeRange {
	p1line := p1.GetLine()
	p2line := p2.GetLine()
	p1col := p1.GetColumn()
	p2col := p2.GetColumn()
	if p1line < p2line || (p1line == p2line && p1col < p2col) {
		return &codeRange{start: p1, end: p2}
	}
	return &codeRange{start: p2, end: p1}
}

func (r *codeRange) SetEditor(editor *MemEditor) {
	r.editor = editor
	r.startOffset = editor.GetOffsetByPosition(r.start)
	r.endOffset = editor.GetOffsetByPosition(r.end)
}

func (r *codeRange) GetEditor() *MemEditor {
	return r.editor
}

func (r *codeRange) GetStart() PositionIf {
	return r.start
}

func (r *codeRange) GetStartOffset() int {
	return r.startOffset
}

func (r *codeRange) GetEnd() PositionIf {
	return r.end
}

func (r *codeRange) GetEndOffset() int {
	return r.endOffset
}

func (p *codeRange) GetTextContext(n int) string {
	if p.editor == nil {
		log.Warn("range.editor is nil")
		return ""
	}
	result, err := p.editor.GetContextAroundRange(p.GetStart(), p.GetEnd(), n)
	if err != nil {
		log.Warnf("editor.GetContextAroundRange(start, end, %v) failed: %v", n, err)
		return ""
	}
	return result
}

func (p *codeRange) GetText() string {
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

func (r *codeRange) Len() int {
	return r.endOffset - r.startOffset + 1
}

func (r *codeRange) Add(end RangeIf) {
	if r.editor == nil {
		log.Warn("range.editor is nil")
		return
	}
	r.end = end.GetEnd()
	r.text = r.editor.GetTextFromOffset(r.startOffset, r.endOffset)
}

func (p codeRange) String() string {
	return fmt.Sprintf(
		"%s - %s: %s",
		p.start, p.end, p.GetText(),
	)
}

func (p *codeRange) GetTextContextWithPrompt(n int, msg ...string) string {
	if p == nil || p.editor == nil {
		log.Warn("range or range.editor is nil")
		return ""
	}
	return p.editor.GetTextContextWithPrompt(p, n, msg...)
}

func (p *codeRange) GetWordText() string {
	if p.editor == nil {
		log.Warn("range.editor is nil")
		return ""
	}
	return p.editor.GetWordTextFromRange(p)
}
