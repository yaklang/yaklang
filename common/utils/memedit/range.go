package memedit

type RangeIf interface {
	GetStart() PositionIf
	GetEnd() PositionIf
}

type PositionIf interface {
	GetOffset() int
	GetLine() int
	GetColumn() int
}

type Position struct {
	line   int
	column int
	offset int
}

func NewPosition(line, column, offset int) *Position {
	return &Position{line: line, column: column, offset: offset}
}

func (p *Position) GetOffset() int {
	return p.offset
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

func NewRange(start, end PositionIf) *Range {
	return &Range{start: start, end: end}
}

func (r *Range) GetStart() PositionIf {
	return r.start
}

func (r *Range) GetEnd() PositionIf {
	return r.end
}
