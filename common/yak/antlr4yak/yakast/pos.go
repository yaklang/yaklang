package yakast

type position struct {
	Line   int
	Column int
}

func (p *position) GT(n *position) bool {
	if p == nil {
		return false
	}

	if p.Line > n.Line {
		return true
	}

	if p.Line < n.Line {
		return false
	}

	if p.Line == n.Line {
		return p.Column > n.Column
	}

	return false
}

func (p *position) GTEQ(n *position) bool {
	if p == nil {
		return false
	}

	if p.Line > n.Line {
		return true
	}

	if p.Line < n.Line {
		return false
	}

	if p.Line == n.Line {
		return p.Column >= n.Column
	}

	return false
}

func inRange(p *position, start *position, end *position) bool {
	if end == nil && start == nil {
		return false
	}

	if end == nil {
		return p.GTEQ(start)
	}

	if start == nil {
		return end.GTEQ(p)
	}

	return p.GTEQ(start) && end.GTEQ(p)
}
