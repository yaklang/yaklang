package ssautil

import (
	"fmt"
	"slices"
	"strings"
)

// ====== value
type value interface {
	Replace(value, value)
	String() string
}
type phi struct {
	edge []value
}

func NewPhi(edge ...value) *phi {
	return &phi{
		edge: edge,
	}
}

func (p *phi) Replace(old, new value) {
	for i, v := range p.edge {
		if v == old {
			p.edge[i] = new
		}
	}
}

func (p *phi) String() string {
	return fmt.Sprintf("phi%s", p.edge)
}

type consts struct {
	value any
}

func NewConsts(value any) *consts {
	return &consts{
		value: value,
	}
}

func (c *consts) Replace(old, new value) {}

func (c *consts) String() string {
	return fmt.Sprintf("const(%v)", c.value)
}

type binary struct {
	X, Y value
}

func NewBinary(x, y value) *binary {
	return &binary{
		X: x,
		Y: y,
	}
}

func (b *binary) Replace(old, new value) {
	if b.X == old {
		b.X = new
	}
	if b.Y == old {
		b.Y = new
	}
}

func (b *binary) String() string {
	return fmt.Sprintf("binary(%v, %v)", b.X, b.Y)
}

// ======== builder

// ssa.cfg like this build function

func buildSyntaxBlock(scope *ScopedVersionedTable[value], buildBody func(*ScopedVersionedTable[value])) {
	/*
		scope [a=1; b=1]
			sub [a=2; b:=2]
		- cover
	*/
	sub := scope.CreateSubScope()
	buildBody(sub)
	scope.CoverByChild()
}

func GeneratePhi(name string, t []value) value {
	slices.SortFunc(t, func(i, j value) int {
		return strings.Compare(i.String(), j.String())
	})
	return NewPhi(t...)
}

func SpinHandler(name string, current, origin, last value) (ret value) {
	if origin == last {
		return last
	}
	// if different value, create phi
	if phi, ok := current.(*phi); ok {
		phi.edge = append(phi.edge, origin)
		phi.edge = append(phi.edge, last)
		return phi
	}
	panic("this value not phi")
}

func NewPhiValue() value {
	return NewPhi()
}
