package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckCanMemberCallExistPhiSelfEdgeDoesNotOverflow(t *testing.T) {
	_, builder := newTestBuilder(t)

	key := builder.EmitConstInst("field")
	base := builder.EmitEmptyContainer()

	phi := builder.EmitPhi("phi", Values{base})
	phi.Edge = append(phi.Edge, phi.GetId())

	res := checkCanMemberCallExist(phi, key)
	require.True(t, res.exist)
	require.NotEmpty(t, res.name)
}

func TestCheckCanMemberCallExistOrTypeDoesNotMutateValueType(t *testing.T) {
	_, builder := newTestBuilder(t)

	key := builder.EmitConstInst("field")
	value := builder.EmitEmptyContainer()

	originalType := NewOrType(CreateStringType(), NewObjectType())
	value.SetType(originalType)

	_ = checkCanMemberCallExist(value, key)

	require.Equal(t, OrTypeKind, value.GetType().GetTypeKind(), "member-call checks should not mutate value types")
}

// isEmpty is a test-only view of the guard's state.
func (s *memberCallReadVisitSet) isEmpty() bool {
	return s.length == 0 && len(s.overflow) == 0
}

func TestMemberCallReadVisitSetInlineStackAndOverflow(t *testing.T) {
	key := func(objectID int64) memberCallReadVisitKey {
		return memberCallReadVisitKey{objectID: objectID, keyID: 1}
	}

	var set memberCallReadVisitSet

	// Within the inline capacity the set must not allocate its overflow map.
	for i := 0; i < memberCallReadVisitStackDepth; i++ {
		require.False(t, set.push(key(int64(i))), "frame %d should stay inline", i)
		require.True(t, set.contains(key(int64(i))))
	}
	require.Zero(t, len(set.overflow), "inline frames must not allocate the overflow map")

	// One frame deeper than the capacity spills into the overflow map only.
	require.True(t, set.push(key(99)))
	require.Equal(t, 1, len(set.overflow))
	require.True(t, set.contains(key(99)))

	// The outermost key re-entered one frame deeper: it is now stored twice, once
	// inline (the original frame) and once in the overflow map (this frame).
	outermost := key(0)
	require.True(t, set.push(outermost))
	require.True(t, set.contains(outermost))

	// Popping the deep frame removes only its overflow entry; the inline copy
	// belongs to the outer frame and must survive.
	set.pop(outermost, true)
	require.True(t, set.contains(outermost), "inline copy must survive the overflow pop")
	set.pop(key(99), true)
	require.False(t, set.contains(key(99)))
	require.Zero(t, len(set.overflow), "overflow map is drained")

	// Unwind in LIFO order, exactly like the recursive reads do: the outermost
	// key is popped last, when it reaches the top of the inline stack.
	for i := memberCallReadVisitStackDepth - 1; i >= 1; i-- {
		require.True(t, set.contains(key(int64(i))))
		set.pop(key(int64(i)), false)
	}
	require.True(t, set.contains(outermost))
	set.pop(outermost, false)
	require.False(t, set.contains(outermost))
	require.True(t, set.isEmpty())
}

func TestMemberCallReadVisitSetIsEmptyOnFreshSet(t *testing.T) {
	var set memberCallReadVisitSet
	require.True(t, set.isEmpty())
	require.False(t, set.contains(memberCallReadVisitKey{objectID: 1, keyID: 1}))
}
