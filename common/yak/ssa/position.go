package ssa

import "fmt"

type Range struct {
	SourceCode *string
	Start, End *Position
}

func NewRange(start, end *Position, source string) *Range {
	return &Range{
		SourceCode: &source,
		Start:      start,
		End:        end,
	}
}

type Position struct {
	Offset int64
	Line   int64
	Column int64
}

func NewPosition(offset, line, column int64) *Position {
	return &Position{
		Offset: offset,
		Line:   line,
		Column: column,
	}
}

// if ret <  0: p before other
// if ret == 0: p = other
// if ret >  0: p after other
func (p *Range) CompareStart(other *Range) int {
	return p.Start.Compare(other.Start)
}
func (p *Range) CompareEnd(other *Range) int {
	return p.End.Compare(other.End)
}

func (p *Position) Compare(other *Position) int {
	return int(p.Offset - other.Offset)
}

func (p *Range) String() string {
	return fmt.Sprintf(
		"%s - %s: %s",
		p.Start, p.End, *p.SourceCode,
	)
}

func (p *Position) String() string {
	return fmt.Sprintf(
		"%3d:%-3d(%3d)",
		p.Line, p.Column, p.Offset,
	)
}
