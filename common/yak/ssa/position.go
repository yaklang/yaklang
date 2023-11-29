package ssa

type Position struct {
	SourceCode  string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
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
