package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
)

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
		"%d:%d",
		p.Line, p.Column,
	)
}

const (
	OFFSET_MASK = 100
)

func (prog *Program) SetOffset(inst Instruction) {
	value, ok := ToValue(inst)
	if !ok {
		return
	}
	set := func(offset int64) {
		mask := offset - offset%OFFSET_MASK
		prog.Offset[mask] = append(prog.Offset[mask], &OffsetValue{
			Offset: offset,
			Values: value,
		})
		log.Infof("SetOffset: %s %d", value, offset)
	}

	if r := value.GetRange(); r != nil {
		// TODO : check if r.Start/End is nil
		set(r.Start.Offset)
		set(r.End.Offset)

	}
}

func (prog *Program) SetOffsetByRange(value Value, r *Range) {
	set := func(offset int64) {
		mask := offset - offset%OFFSET_MASK
		prog.Offset[mask] = append(prog.Offset[mask], &OffsetValue{
			Offset: offset,
			Values: value,
		})
		log.Infof("SetOffset: %s %d", value, offset)
	}

	// TODO : check if r.Start/End is nil
	set(r.Start.Offset)
	set(r.End.Offset)
}

func (prog *Program) GetValuesByOffset(offset int64) []*OffsetValue {

	mask := offset - offset%OFFSET_MASK

	if vs, ok := prog.Offset[mask]; ok {
		return vs
	}
	return nil
}
