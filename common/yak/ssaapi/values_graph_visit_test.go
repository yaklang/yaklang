package ssaapi

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestValueDFSVisitsDiamondPerPath(t *testing.T) {
	prog, err := Parse("", WithProgramName("dfs-"+uuid.NewString()), WithLanguage(ssaconfig.Yak))
	require.NoError(t, err)

	newValue := func(id int64) *Value {
		inst := ssa.NewConst(id)
		inst.SetId(id)
		v, err := prog.NewValue(inst)
		require.NoError(t, err)
		return v
	}
	root, left, right, shared := newValue(1), newValue(2), newValue(3), newValue(4)
	seen := make(map[*Value]int, 4)
	err = valueDFS(root, func(v *Value) (Values, error) {
		seen[v]++
		switch v {
		case root:
			return Values{left, right}, nil
		case left, right:
			return Values{shared}, nil
		default:
			return nil, nil
		}
	}, context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, seen[root])
	require.Equal(t, 1, seen[left])
	require.Equal(t, 1, seen[right])
	// DFS only remembers ancestors on the current path; the two paths reach
	// the shared node independently. Graph consumers deduplicate edges instead
	// of changing traversal semantics.
	require.Equal(t, 2, seen[shared])
}
