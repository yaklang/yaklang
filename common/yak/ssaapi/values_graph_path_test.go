package ssaapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestDataflowPathsKeepDistinctAnalysisValues(t *testing.T) {
	for _, direction := range []string{"effect", "depend"} {
		t.Run(direction, func(t *testing.T) {
			program, err := ssaapi.Parse(`root = start(); shared = branch(); left = allowed(); right = blocked()`)
			require.NoError(t, err)
			value := func(name string) *ssaapi.Value {
				values := program.SyntaxFlowChain(name)
				require.Len(t, values, 1)
				return values[0]
			}
			root, shared, left, right := value("root"), value("shared"), value("left"), value("right")
			first := shared.NewValue(shared.GetSSAInst())
			second := shared.NewValue(shared.GetSSAInst())
			require.Equal(t, first.GetId(), second.GetId())
			require.NotEqual(t, first.GetUID(), second.GetUID())
			connect := func(from *ssaapi.Value, to ...*ssaapi.Value) {
				edges := utils.NewSafeMapWithKey[int64, *ssaapi.Value]()
				for _, next := range to {
					edges.Set(next.GetUID(), next)
				}
				if direction == "effect" {
					from.EffectOn = edges
				} else {
					from.DependOn = edges
				}
			}
			connect(root, first, second)
			connect(first, left)
			connect(second, right)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			paths := root.GetDataflowPathWithContext(ctx)
			require.Len(t, paths, 2, "two analysis values may share an instruction but carry different continuations")
			terminals := map[int64]bool{}
			for _, path := range paths {
				for _, node := range path {
					terminals[node.GetUID()] = true
				}
			}
			require.True(t, terminals[left.GetUID()])
			require.True(t, terminals[right.GetUID()])
			filtered := root.GetDataflowPathWithEdgeFilterWithContext(ctx, func(_, to *ssaapi.Value) bool { return to != right })
			require.Len(t, filtered, 2, "filtering one continuation must not discard its sibling")
			// A real cycle still terminates while the other branch remains reachable.
			connect(first, root)
			paths = root.GetDataflowPathWithContext(ctx)
			require.Len(t, paths, 1)
			require.NoError(t, ctx.Err())
			require.Contains(t, paths[0], right)
		})
	}
}
