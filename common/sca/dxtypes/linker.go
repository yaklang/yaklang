package dxtypes

import "github.com/yaklang/yaklang/common/sca/core/budget"

// Linker caches identities only for one immutable graph construction phase.
// Discard it before changing any Package identity field. Package.Identifier
// itself stays uncached, so callers can mutate public fields without stale IDs.
type Linker struct {
	state *budget.State
	ids   map[*Package]string
}

func NewLinker(st *budget.State) *Linker { return &Linker{state: st} }

func (l *Linker) identifier(p *Package) (string, error) {
	if id, ok := l.ids[p]; ok {
		return id, nil
	}
	if err := l.state.Working(budget.SizeMap + budget.SizePtr + 64 + budget.SizeString); err != nil {
		return "", err
	}
	if l.ids == nil {
		l.ids = make(map[*Package]string)
	}
	id := p.Identifier()
	l.ids[p] = id
	return id, nil
}

func (l *Linker) Link(down, up *Package) error {
	downID, err := l.identifier(down)
	if err != nil {
		return err
	}
	upID, err := l.identifier(up)
	if err != nil {
		return err
	}
	// Reserve both map insertions before either mutation.
	if err := l.state.Working(2 * (budget.SizeMap + budget.SizeString + budget.SizePtr)); err != nil {
		return err
	}
	if up.DownStreamPackages == nil {
		up.DownStreamPackages = make(map[string]*Package)
	}
	if down.UpStreamPackages == nil {
		down.UpStreamPackages = make(map[string]*Package)
	}
	up.DownStreamPackages[downID] = down
	down.UpStreamPackages[upID] = up
	return nil
}
