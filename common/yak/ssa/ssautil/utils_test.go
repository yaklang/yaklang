package ssautil

import (
	"fmt"
)

// ====== value
type value interface {
	Replace(value, value)
	String() string
	IsUndefined() bool
	IsParameter() bool
	IsSideEffect() bool
	IsPhi() bool
	SelfDelete()
	GetId() int64
}

type phi struct {
	edge []value

	visited bool
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
	if p.visited {
		return "self"
	} else {
		p.visited = true
		defer func() {
			p.visited = false
		}()
	}
	return fmt.Sprintf("phi%s", p.edge)
}

func (p *phi) IsUndefined() bool  { return false }
func (p *phi) IsParameter() bool  { return false }
func (p *phi) IsSideEffect() bool { return false }
func (p *phi) IsPhi() bool        { return true }
func (p *phi) SelfDelete()        {}

type constsIns struct {
	value any
}

func (c *constsIns) GetId() int64 {
	return 0
}

func NewConsts(value any) *constsIns {
	return &constsIns{
		value: value,
	}
}

func (c *constsIns) Replace(old, new value) {}

func (c *constsIns) String() string {
	return fmt.Sprintf("const(%v)", c.value)
}
func (p *constsIns) IsUndefined() bool  { return false }
func (p *constsIns) IsParameter() bool  { return false }
func (p *constsIns) IsSideEffect() bool { return false }
func (p *constsIns) IsPhi() bool        { return false }
func (p *constsIns) SelfDelete()        {}

type binary struct {
	X, Y value
}

func (b *binary) GetId() int64 {
	return 0
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

func (p *binary) IsUndefined() bool  { return false }
func (p *binary) IsParameter() bool  { return false }
func (p *binary) IsSideEffect() bool { return false }
func (p *binary) IsPhi() bool        { return false }
func (p *binary) SelfDelete()        {}

// ======== builder

func GeneratePhi(name string, t []value) value {
	return NewPhi(t...)
}

func SpinHandler(name string, current, origin, last value) map[string]value {
	ret := make(map[string]value)
	if origin == last {
		ret[name] = origin
	}
	// if different value, create phi
	if phi, ok := current.(*phi); ok {
		phi.edge = append(phi.edge, origin)
		phi.edge = append(phi.edge, last)
		// return phi
		ret[name] = phi
	}
	// panic("this value not phi")
	return ret
}

func NewPhiValue(name string) value {
	return NewPhi()
}

var _ ScopedVersionedTableIF[value] = (*ScopedVersionedTable[value])(nil)

var _ LabelTarget[value] = (*LoopStmt[value])(nil)
var _ LabelTarget[value] = (*SwitchStmt[value])(nil)

func (v *phi) GetId() int64 {
	return 0
}
