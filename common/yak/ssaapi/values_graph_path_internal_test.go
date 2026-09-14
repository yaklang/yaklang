package ssaapi

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func pathTestValue(t *testing.T, p *Program, id int64) *Value {
	t.Helper()
	inst := ssa.NewConst(id)
	inst.SetId(id)
	v, err := p.NewValue(inst)
	require.NoError(t, err)
	return v
}

func TestDataflowPathsAcrossPrograms(t *testing.T) {
	for _, direction := range []string{"effect", "depend"} {
		t.Run(direction, func(t *testing.T) {
			p, q := NewTmpProgram("path-left"), NewTmpProgram("path-right")
			root, other := pathTestValue(t, p, 10), pathTestValue(t, q, 20)
			left, right := pathTestValue(t, p, 30), pathTestValue(t, q, 40)
			connect, remove := (*Value).AppendEffectOn, (*Value).RemoveEffectOn
			paths := (*Value).GetEffectOnPath
			if direction == "depend" {
				connect, remove = (*Value).AppendDependOn, (*Value).RemoveDependOn
				paths = (*Value).GetDependOnPath
			}
			// The first values of two Programs used to share UID 1, so even
			// this direct edge was mistaken for a self edge.
			connect(root, other)
			connect(other, left)
			connect(other, right)
			connect(other, right) // adding one edge twice must remain idempotent
			require.ElementsMatch(t, []Values{{other, left}, {other, right}}, paths(root))
			remove(other, left)
			require.Equal(t, []Values{{other, right}}, paths(root))
			connect(right, root) // a real cross-Program cycle still terminates
			require.Empty(t, paths(root))
		})
	}
}

func TestDataflowPathsReloadedAuditCycle(t *testing.T) {
	db := ssadb.GetDB()
	require.NoError(t, db.AutoMigrate(&ssadb.AuditNode{}, &ssadb.AuditEdge{}).Error)
	p := NewTmpProgram("path-audit-" + ssadb.NewULID())
	node := func(name string) *ssadb.AuditNode {
		n := ssadb.NewAuditNode()
		n.ProgramName, n.IRCodeID, n.TmpValue = p.GetProgramName(), -1, name
		require.NoError(t, db.Create(n).Error)
		return n
	}
	a, b, leaf := node("root"), node("cycle"), node("leaf")
	t.Cleanup(func() {
		db.Unscoped().Where("program_name = ?", p.GetProgramName()).Delete(&ssadb.AuditEdge{})
		db.Unscoped().Where("program_name = ?", p.GetProgramName()).Delete(&ssadb.AuditNode{})
	})
	for _, kind := range []ssadb.AuditEdgeType{ssadb.EdgeType_EffectsOn, ssadb.EdgeType_DependsOn} {
		for _, pair := range [][2]*ssadb.AuditNode{{a, b}, {b, a}, {a, leaf}} {
			require.NoError(t, db.Create(&ssadb.AuditEdge{
				ProgramName: p.GetProgramName(), FromNode: pair[0].NodeID, ToNode: pair[1].NodeID, EdgeType: kind,
			}).Error)
		}
	}
	for _, direction := range []string{"effect", "depend"} {
		t.Run(direction, func(t *testing.T) {
			p.nodeId2ValueCache.Purge()
			root := p.NewValueFromAuditNode(db, a.NodeID)
			require.NotNil(t, root)
			p.nodeId2ValueCache.Purge()
			clone := p.NewValueFromAuditNode(db, a.NodeID)
			require.NotSame(t, root, clone)
			require.NotEqual(t, root.GetUID(), clone.GetUID())
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			// Force cache eviction throughout the walk, without waiting for TTL.
			filter := func(_, _ *Value) bool { p.nodeId2ValueCache.Purge(); return true }
			var paths []Values
			if direction == "effect" {
				paths = root.GetEffectOnPathWithEdgeFilterWithContext(ctx, filter)
			} else {
				paths = root.GetDependOnPathWithEdgeFilterWithContext(ctx, filter)
			}
			require.NoError(t, ctx.Err(), "reloaded nodes must close the cycle before the deadline")
			require.Len(t, paths, 1)
			require.Len(t, paths[0], 1)
			require.Equal(t, leaf.NodeID, paths[0][0].auditNode.NodeID)
			visited := map[string]int{}
			require.NoError(t, valueDFS(root, func(v *Value) (Values, error) {
				visited[v.auditNode.NodeID]++
				p.nodeId2ValueCache.Purge()
				if direction == "effect" {
					return v.GetEffectOn(), nil
				}
				return v.GetDependOn(), nil
			}, ctx))
			require.Equal(t, map[string]int{a.NodeID: 1, b.NodeID: 1, leaf.NodeID: 1}, visited)
		})
	}
}

// Two wrappers for the same instruction at each layer produce 2^depth paths.
func pathTestDiamonds(t *testing.T, depth int) *Value {
	t.Helper()
	p := NewTmpProgram("path-diamonds")
	root := pathTestValue(t, p, 1)
	previous := Values{root}
	for layer := 0; layer < depth; layer++ {
		a, b := pathTestValue(t, p, int64(layer+10)), pathTestValue(t, p, int64(layer+10))
		for _, v := range previous {
			v.AppendEffectOn(a).AppendEffectOn(b)
		}
		previous = Values{a, b}
	}
	return root
}

func TestDataflowFilterPathBudget(t *testing.T) {
	for _, matches := range []bool{true, false} {
		name := "no_match_exhausts_budget"
		if matches {
			name = "first_match_short_circuits"
		}
		t.Run(name, func(t *testing.T) {
			root := pathTestDiamonds(t, 16)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			budget := sf.NewRuleWorkBudget(100, cancel)
			cfg := sf.NewConfig(sf.WithContext(ctx), sf.WithWorkBudget(budget))
			result := sf.NewSFResult(nil, cfg)
			if matches {
				result.SymbolTable.Set("source", ToSFVMValues(Values{root}))
			}
			got := dataFlowFilter(ctx, Values{root}, result, cfg, nil, nil, nil,
				withFilterCondition(sf.RecursiveConfig_Include, "* & $source"))
			if matches {
				require.Equal(t, Values{root}, got)
				require.Greater(t, budget.Visited(), int64(16), "the first path's node visits must count as work")
				require.False(t, budget.Exceeded(), "matching the first path must not enumerate the remaining 65535")
				require.NoError(t, ctx.Err())
			} else {
				require.Empty(t, got)
				require.True(t, budget.Exceeded(), "path expansion must consume the rule budget")
				require.ErrorIs(t, ctx.Err(), context.Canceled)
			}
		})
	}
}

func TestDataflowFilterCallerCancellation(t *testing.T) {
	root := pathTestDiamonds(t, 3)
	cfg := sf.NewConfig()
	result := sf.NewSFResult(nil, cfg)
	result.SymbolTable.Set("source", ToSFVMValues(Values{root}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Empty(t, dataFlowFilter(ctx, Values{root}, result, cfg, nil, nil, nil,
		withFilterCondition(sf.RecursiveConfig_Include, "* & $source")))
}

func TestDataflowNativeCallCancellationIsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := sf.NewConfig(sf.WithContext(ctx))
	vm := sf.NewSyntaxFlowVirtualMachine()
	vm.SetConfig(cfg)
	frame, err := vm.Compile("*")
	require.NoError(t, err)
	frame.SetSFResult(sf.NewSFResult(nil, cfg))
	ok, _, err := nativeCallDataFlow(ToSFVMValues(Values{pathTestDiamonds(t, 2)}), frame,
		sf.NewNativeCallActualParams(&sf.RecursiveConfigItem{Key: "exclude", Value: "* & $source"}))
	require.False(t, ok)
	require.ErrorIs(t, err, sf.CriticalError, "a cancelled native call must not become a successful empty result")
	require.ErrorContains(t, err, "dataflow path traversal interrupted")
}

func TestAnalysisValueUIDAcrossConcurrentPrograms(t *testing.T) {
	const programs, valuesPerProgram = 8, 128
	inst := ssa.NewConst(42)
	inst.SetId(42)
	values := make(chan *Value, programs*valuesPerProgram)
	var wg sync.WaitGroup
	for i := 0; i < programs; i++ {
		p := NewTmpProgram("concurrent-value-ids")
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < valuesPerProgram; j++ {
				v, err := p.NewValue(inst)
				if err != nil {
					t.Error(err)
					return
				}
				values <- v
			}
		}()
	}
	wg.Wait()
	close(values)
	seen := make(map[int64]bool)
	for v := range values {
		require.Equal(t, int64(42), v.GetId(), "SSA identity must remain unchanged")
		require.Positive(t, v.GetUID())
		require.False(t, seen[v.GetUID()], "independent analysis values must not collide across Programs")
		seen[v.GetUID()] = true
	}
	require.Len(t, seen, programs*valuesPerProgram)
}

func TestDataflowPathVisitor(t *testing.T) {
	p := NewTmpProgram("path-product")
	root := pathTestValue(t, p, 1)
	a, b := pathTestValue(t, p, 2), pathTestValue(t, p, 3)
	c, d := pathTestValue(t, p, 4), pathTestValue(t, p, 5)
	root.AppendEffectOn(a).AppendEffectOn(b)
	root.AppendDependOn(c).AppendDependOn(d)
	// Two effect paths and two depend paths retain the legacy orientation.
	want := []Values{{a, root, c}, {a, root, d}, {b, root, c}, {b, root, d}}
	require.ElementsMatch(t, want, root.GetDataflowPath())
	require.ElementsMatch(t, []Values{{a, root, c}, {a, root, d}}, root.GetDataflowPathWithEdgeFilter(func(_, to *Value) bool { return to != b }))
	require.Equal(t, []Values{{a, root}}, root.GetDataflowPath(a))
	require.Equal(t, []Values{{root, c}}, root.GetDataflowPath(c))
	require.Empty(t, root.GetDataflowPath(root))

	ctx, cancel := context.WithCancel(context.Background())
	visits := 0
	require.False(t, root.visitDataflowPaths(ctx, nil, nil, func(path Values) bool {
		visits++
		cancel() // cancellation during the product must stop subsequent output
		return true
	}))
	require.Equal(t, 1, visits)

	diamonds := pathTestDiamonds(t, 24) // 16 million possible paths
	work, matches := 0, 0
	require.False(t, diamonds.visitDataflowPaths(context.Background(), nil, func() bool {
		work++
		return work > 100
	}, func(path Values) bool {
		matches++
		require.Len(t, path, 25)
		return false
	}))
	require.Equal(t, 1, matches)
	require.Less(t, work, 100, "first-match filtering must keep work proportional to path depth")
}
