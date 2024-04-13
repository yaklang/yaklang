package memedit

type RangeIf interface {
	GetStart() PositionIf
	GetEnd() PositionIf
}

type PositionIf interface {
	GetLine() int
	GetColumn() int
}

type Position struct {
	line   int
	column int
}

func NewPosition(line, column int) *Position {
	return &Position{line: line, column: column}
}

func (p *Position) GetLine() int {
	return p.line
}

func (p *Position) GetColumn() int {
	return p.column
}

type Range struct {
	start PositionIf
	end   PositionIf
}

func NewRange(p1, p2 PositionIf) *Range {
	p1line := p1.GetLine()
	p2line := p2.GetLine()
	p1col := p1.GetColumn()
	p2col := p2.GetColumn()
	if p1line < p2line || (p1line == p2line && p1col < p2col) {
		return &Range{start: p1, end: p2}
	}
	return &Range{start: p2, end: p1}
}

func (r *Range) GetStart() PositionIf {
	return r.start
}

func (r *Range) GetEnd() PositionIf {
	return r.end
}
