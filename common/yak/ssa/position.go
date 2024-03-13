package ssa

import (
	"fmt"

	"github.com/samber/lo"
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
	OFFSET_SEGMENT = 128
)

func (prog *Program) setOffsetForValue(offset int64, value Value) {
	mask := offset - offset%OFFSET_SEGMENT
	m, ok := prog.OffsetSegmentToValues[mask]
	if !ok {
		m = make(map[int64]*OffsetValues)
		prog.OffsetSegmentToValues[mask] = m
	}

	vs, ok := m[offset]
	if !ok {
		m[offset] = &OffsetValues{
			Offset: offset,
			Values: Values{value},
		}
	} else {
		vs.Values = lo.Uniq(append(vs.Values, value))
	}
}

func (prog *Program) SetOffset(inst Instruction) {
	value, ok := ToValue(inst)
	if !ok {
		return
	}

	if r := value.GetRange(); r != nil {
		var startOffset int64 = -1
		if r.Start != nil {
			startOffset = r.Start.Offset
			// log.Infof("value=%s range=%s offset=%d", value.String(), r, startOffset)
			prog.setOffsetForValue(r.Start.Offset, value)
		}
		if r.End != nil && startOffset != r.End.Offset {
			// log.Infof("value=%s range=%s offset=%d", value.String(), r, r.End.Offset)
			prog.setOffsetForValue(r.End.Offset, value)
		}
	}
}

func (prog *Program) SetOffsetByRange(r *Range, value Value) {
	if r == nil {
		return
	}
	var startOffset int64 = -1
	if r.Start != nil {
		startOffset = r.Start.Offset
		// log.Infof("value=%s range=%s offset=%d", value.String(), r, startOffset)
		prog.setOffsetForValue(r.Start.Offset, value)
	}
	if r.End != nil && startOffset != r.End.Offset {
		// log.Infof("value=%s range=%s offset=%d", value.String(), r, r.End.Offset)
		prog.setOffsetForValue(r.End.Offset, value)
	}
}

func (prog *Program) GetValuesMapByOffset(offset int64) map[int64]*OffsetValues {
	mask := offset - offset%OFFSET_SEGMENT

	if m, ok := prog.OffsetSegmentToValues[mask]; ok {
		return m
	}
	return nil
}
