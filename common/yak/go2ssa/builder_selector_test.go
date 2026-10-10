package go2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestResolveGoEmbeddedFieldCycle(t *testing.T) {
	model := ssa.NewStructType()
	model.AddField(ssa.NewConst("ID"), ssa.CreateNumberType())
	flow := ssa.NewStructType()
	flow.AddField(ssa.NewConst("Model"), goPointerType(model))
	flow.AnonymousField["Model"] = model
	root := ssa.NewStructType()
	root.AddField(ssa.NewConst("Loop"), goPointerType(root))
	root.AnonymousField["Loop"] = root
	root.AddField(ssa.NewConst("Flow"), flow)
	root.AnonymousField["Flow"] = flow

	t.Run("reachable field beside pointer cycle", func(t *testing.T) {
		selector := resolveGoFieldSelector(root, "ID")
		require.False(t, selector.ambiguous)
		require.Equal(t, []string{"Flow", "Model", "ID"}, selector.path)
	})
	t.Run("missing field terminates", func(t *testing.T) {
		selector := resolveGoFieldSelector(root, "Missing")
		require.False(t, selector.ambiguous)
		require.Empty(t, selector.path)
	})
	t.Run("shallower field wins over recursive routes", func(t *testing.T) {
		root.AddField(ssa.NewConst("ID"), ssa.CreateStringType())
		selector := resolveGoFieldSelector(root, "ID")
		require.False(t, selector.ambiguous)
		require.Equal(t, []string{"ID"}, selector.path)
	})
}
