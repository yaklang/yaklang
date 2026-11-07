package ssa

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// emitPhiDiamond builds depth nested diamonds that reconverge on the same
// previous phi:
//
//	prev <- phi(left, right); left = phi(prev, a); right = phi(prev, b)
//
// Reading a member of the top phi expands prev once per path, so a
// depth-only recursion guard costs 2^depth reads of the same node.
func emitPhiDiamond(builder *FunctionBuilder, depth int, a, b Value) Value {
	prev := builder.EmitPhi("diamond-0", Values{a, b})
	for i := 1; i <= depth; i++ {
		left := builder.EmitPhi(fmt.Sprintf("diamond-%d-left", i), Values{prev, a})
		right := builder.EmitPhi(fmt.Sprintf("diamond-%d-right", i), Values{prev, b})
		prev = builder.EmitPhi(fmt.Sprintf("diamond-%d", i), Values{left, right})
	}
	return prev
}

func countBlockPhis(builder *FunctionBuilder) int {
	return len(builder.CurrentBlock.Phis)
}

// A member read of a reconverging phi DAG must stay linear in the number of
// distinct nodes. Before per-read result reuse it emitted one phi per path,
// which is 2^depth for the diamond above and made minified JS bundles spin for
// tens of minutes.
func TestReadPhiMemberDAGKeepsLinearWork(t *testing.T) {
	const depth = 12

	_, builder := newTestBuilder(t)
	key := builder.EmitConstInst("field")
	a := emitObject(builder, "a")
	b := emitObject(builder, "b")
	aMember := builder.EmitConstInst("from-a")
	bMember := builder.EmitConstInst("from-b")
	setMemberCallRelationship(a, key, aMember)
	setMemberCallRelationship(b, key, bMember)

	top := emitPhiDiamond(builder, depth, a, b)
	before := countBlockPhis(builder)

	read := builder.ReadMemberCallValue(top, key)
	require.NotNil(t, read)

	emitted := countBlockPhis(builder) - before
	// With result reuse each distinct phi node is read once and contributes a
	// bounded number of emitted phis. 2^depth (4096) does not fit.
	require.LessOrEqual(t, emitted, 4*depth,
		"reading a reconverging phi DAG must not emit one phi per path; depth=%d emitted=%d", depth, emitted)

	// The result must still reach both leaf members.
	seen := map[int64]struct{}{}
	var walk func(value Value, visit map[int64]struct{})
	walk = func(value Value, visit map[int64]struct{}) {
		if value == nil {
			return
		}
		if _, ok := visit[value.GetId()]; ok {
			return
		}
		visit[value.GetId()] = struct{}{}
		seen[value.GetId()] = struct{}{}
		if phi, ok := ToPhi(value); ok {
			for _, edge := range phi.GetValues() {
				walk(edge, visit)
			}
		}
	}
	walk(read, map[int64]struct{}{})
	_, hasA := seen[aMember.GetId()]
	_, hasB := seen[bMember.GetId()]
	require.True(t, hasA, "read result must reach the member of the first leaf")
	require.True(t, hasB, "read result must reach the member of the second leaf")
}

// Result reuse is scoped to a single read. A later write that changes a member
// must be visible to the next read; caching across writes would resurrect a
// stale value.
func TestReadPhiMemberSeesMemberWritesBetweenReads(t *testing.T) {
	_, builder := newTestBuilder(t)
	key := builder.EmitConstInst("field")
	a := emitObject(builder, "a")
	b := emitObject(builder, "b")
	oldMember := builder.EmitConstInst("old")
	newMember := builder.EmitConstInst("new")
	setMemberCallRelationship(a, key, oldMember)
	setMemberCallRelationship(b, key, oldMember)

	top := emitPhiDiamond(builder, 2, a, b)
	first := builder.ReadMemberCallValue(top, key)
	require.NotNil(t, first)

	replaced := emitObject(builder, "replaced-a")
	setMemberCallRelationship(replaced, key, newMember)
	setMemberCallRelationship(a, key, newMember)

	second := builder.ReadMemberCallValue(top, key)
	require.NotNil(t, second)

	seen := map[int64]struct{}{}
	var walk func(value Value, visit map[int64]struct{})
	walk = func(value Value, visit map[int64]struct{}) {
		if value == nil {
			return
		}
		if _, ok := visit[value.GetId()]; ok {
			return
		}
		visit[value.GetId()] = struct{}{}
		seen[value.GetId()] = struct{}{}
		if phi, ok := ToPhi(value); ok {
			for _, edge := range phi.GetValues() {
				walk(edge, visit)
			}
		}
	}
	walk(second, map[int64]struct{}{})
	_, hasNew := seen[newMember.GetId()]
	require.True(t, hasNew, "second read must observe the replaced member")
}
