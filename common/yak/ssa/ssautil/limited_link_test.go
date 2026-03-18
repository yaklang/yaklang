package ssautil

import "testing"

type dummySSAValue struct{ id int64 }

func (d *dummySSAValue) IsUndefined() bool  { return false }
func (d *dummySSAValue) IsParameter() bool  { return false }
func (d *dummySSAValue) IsSideEffect() bool { return false }
func (d *dummySSAValue) IsPhi() bool        { return false }
func (d *dummySSAValue) SelfDelete()        {}
func (d *dummySSAValue) GetId() int64       { return d.id }

func TestLinkNodeAllOrderNewestFirst(t *testing.T) {
	var ln LinkNode[*dummySSAValue]

	a := NewVersioned[*dummySSAValue](0, "a", true, nil)
	b := NewVersioned[*dummySSAValue](0, "b", true, nil)
	c := NewVersioned[*dummySSAValue](0, "c", true, nil)

	ln.Append(a)
	ln.Append(b)
	ln.Append(c)

	all := ln.All()
	if len(all) != 3 {
		t.Fatalf("unexpected len(all)=%d", len(all))
	}

	// Expect newest -> oldest (versions: 2,1,0)
	if got := all[0].GetVersion(); got != 2 {
		t.Fatalf("all[0].GetVersion()=%d, want=2", got)
	}
	if got := all[1].GetVersion(); got != 1 {
		t.Fatalf("all[1].GetVersion()=%d, want=1", got)
	}
	if got := all[2].GetVersion(); got != 0 {
		t.Fatalf("all[2].GetVersion()=%d, want=0", got)
	}
}
