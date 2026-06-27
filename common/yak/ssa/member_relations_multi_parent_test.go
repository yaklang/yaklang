package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetAllObjects_DeduplicatesByValueID covers the ssa-level helper used by
// the multi-parent read-path optimization: a member shared across two objects
// must surface both parents, deduplicated by id, with stable append order.
// Two distinct keys are used so GetAllKeys also returns two entries.
func TestGetAllObjects_DeduplicatesByValueID(t *testing.T) {
	_, builder := newTestBuilder(t)

	obj1 := builder.EmitEmptyContainer()
	obj2 := builder.EmitEmptyContainer()
	key1 := builder.EmitConstInst("k1")
	key2 := builder.EmitConstInst("k2")
	shared := builder.EmitUndefined("shared")

	setMemberCallRelationship(obj1, key1, shared)
	setMemberCallRelationship(obj2, key2, shared)

	objects := GetAllObjects(shared)
	require.Len(t, objects, 2)
	require.ElementsMatch(t,
		[]int64{obj1.GetId(), obj2.GetId()},
		[]int64{objects[0].GetId(), objects[1].GetId()},
	)

	keys := GetAllKeys(shared)
	require.Len(t, keys, 2)
	require.ElementsMatch(t,
		[]int64{key1.GetId(), key2.GetId()},
		[]int64{keys[0].GetId(), keys[1].GetId()},
	)
}

// TestForEachOwnerPair_StopsOnFalse verifies the early-exit contract of
// ForEachOwnerPair so hot-path callers can rely on it to scan only until the
// first matching parent.
func TestForEachOwnerPair_StopsOnFalse(t *testing.T) {
	_, builder := newTestBuilder(t)

	obj1 := builder.EmitEmptyContainer()
	obj2 := builder.EmitEmptyContainer()
	key := builder.EmitConstInst("k")
	shared := builder.EmitUndefined("shared")

	setMemberCallRelationship(obj1, key, shared)
	setMemberCallRelationship(obj2, key, shared)

	count := 0
	ForEachOwnerPair(shared, func(ObjectKeyPair) bool {
		count++
		return false
	})
	require.Equal(t, 1, count, "ForEachOwnerPair must stop on first fn()==false")
}

// TestGetAllMembersByKey_ReturnsEveryMatch covers the alias used by
// checkCanMemberCallExist's bottom fallback: when an object holds several
// members under the same key, every member must be returned (not just the
// latest one) so multi-type merging can collect all candidate types.
func TestGetAllMembersByKey_ReturnsEveryMatch(t *testing.T) {
	_, builder := newTestBuilder(t)

	obj := builder.EmitEmptyContainer()
	key := builder.EmitConstInst("cmd")
	member1 := builder.EmitUndefined("member1")
	member2 := builder.EmitUndefined("member2")

	setMemberCallRelationship(obj, key, member1)
	setMemberCallRelationship(obj, key, member2)

	members := GetAllMembersByKey(obj, key)
	require.Len(t, members, 2)
	require.ElementsMatch(t,
		[]int64{member1.GetId(), member2.GetId()},
		[]int64{members[0].GetId(), members[1].GetId()},
	)
}
