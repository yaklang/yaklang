package ssa

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
)

type Range struct {
	originSourceCodeHash string

	SourceCode       *string
	OriginSourceCode *string
	Start, End       *Position
}

func (r *Range) GetOriginSourceCodeHash() string {
	if r.originSourceCodeHash == "" {
		r.originSourceCodeHash = utils.CalcMd5(*r.OriginSourceCode)
	}
	return r.originSourceCodeHash
}

func NewRange(start, end *Position, source string, origin string) *Range {
	return &Range{
		OriginSourceCode: &origin,
		SourceCode:       &source,
		Start:            start,
		End:              end,
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
		"%d:%d",
		p.Line, p.Column,
	)
}
