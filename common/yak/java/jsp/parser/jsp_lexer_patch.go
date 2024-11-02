package jspparser

func (p *JSPLexer) LA(idx int) int {
	return p.GetInputStream().LA(idx)
}
