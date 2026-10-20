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

func TestLinkNodeFindShortCircuitAndDetachedSnapshots(t *testing.T) {
	var ln LinkNode[*dummySSAValue]
	var empty *LinkNode[*dummySSAValue]
	visits := 0
	match := func(VersionedIF[*dummySSAValue]) bool { visits++; return true }
	if empty.Find(match) != nil || ln.Find(match) != nil || visits != 0 {
		t.Fatal("empty chains must not invoke the predicate")
	}
	a := NewVersioned[*dummySSAValue](0, "x", true, nil)
	b := NewVersioned[*dummySSAValue](1, "x", true, nil)
	ln.Append(a)
	ln.Append(b)
	if ln.Find(match) != b || visits != 1 {
		t.Fatal("matching the latest version must stop immediately")
	}
	snapshot := ln.All()
	snapshot[0] = nil
	if ln.All()[0] != b {
		t.Fatal("returned snapshots must be independent")
	}
	c := NewVersioned[*dummySSAValue](2, "x", true, nil)
	ln.Append(c)
	if len(snapshot) != 2 || snapshot[1] != a || ln.First() != a || ln.Last() != c {
		t.Fatal("append must preserve existing snapshots and first/last identities")
	}
	visits = 0
	if ln.Find(func(v VersionedIF[*dummySSAValue]) bool { visits++; return v == a }) != a || visits != 3 {
		t.Fatal("older matches must be visited newest first")
	}
}

func TestLinkNodeRemoveDropsOnlyTheTarget(t *testing.T) {
	var ln LinkNode[*dummySSAValue]
	a := NewVersioned[*dummySSAValue](0, "x", true, nil)
	b := NewVersioned[*dummySSAValue](1, "x", true, nil)
	c := NewVersioned[*dummySSAValue](2, "x", true, nil)
	ln.Append(a)
	ln.Append(b)
	ln.Append(c)

	if !ln.Remove(b) || len(ln.All()) != 2 || ln.Last() != c || ln.First() != a {
		t.Fatal("removing the middle version must keep newest and oldest")
	}
	if !ln.Remove(a) || len(ln.All()) != 1 || ln.Last() != c || ln.First() != c {
		t.Fatal("removing the oldest version must move the head forward")
	}
	if ln.Remove(b) {
		t.Fatal("a version already removed must not match again")
	}
	if !ln.Remove(c) || ln.All() != nil || ln.Last() != nil || ln.First() != nil {
		t.Fatal("removing the only version must clear the chain")
	}
}
