package ssa

type Position struct {
	SourceCode  string
	StartOffset int64
	StartLine   int
	StartColumn int
	EndOffset   int64
	EndLine     int
	EndColumn   int
}

// if ret <  0: p before other
// if ret == 0: p = other
// if ret >  0: p after other
func (p *Position) CompareStart(other *Position) int {
	return int(p.StartOffset - other.StartOffset)
}
func (p *Position) CompareEnd(other *Position) int {
	return int(p.EndOffset - other.EndOffset)
}

func (p *Position) InPosition(p2 *Position) bool {
	if p.StartLine < p2.StartLine {
		return false
	}
	if p.StartLine == p2.StartLine && p.StartColumn < p2.StartColumn {
		return false
	}
	if p.EndLine > p2.EndLine {
		return false
	}
	if p.EndLine == p2.EndLine && p.EndColumn > p2.EndColumn {
		return false
	}
	return true
}
