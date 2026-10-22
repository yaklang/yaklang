package ssa

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// GetId is the work being counted, rather than a machine-dependent duration.
// Once registered, unrelated members must not be inspected for each rewrite.
type blueprintIDProbe struct {
	Value
	reads atomic.Int64
}

func (v *blueprintIDProbe) GetId() int64 {
	v.reads.Add(1)
	return v.Value.GetId()
}

func blueprintTestValue(id int64) Value {
	v := NewConst(id)
	v.SetId(id)
	return v
}

func TestBlueprintReplacementDoesNotInspectUnrelatedMembers(t *testing.T) {
	bp := NewBlueprint("indexed")
	var unrelated []*blueprintIDProbe
	for i := 0; i < 512; i++ {
		v := &blueprintIDProbe{Value: blueprintTestValue(int64(i + 1))}
		unrelated = append(unrelated, v)
		bp.RegisterNormalMember(fmt.Sprintf("field-%d", i), v, false)
	}
	from, to, third := blueprintTestValue(1001), blueprintTestValue(1002), blueprintTestValue(1003)
	bp.RegisterNormalMember("target", from, false)
	bp.RegisterNormalMember("target", third, false)
	bp.RegisterNormalMember("target", from, false)
	before, _ := bp.NormalMember.Get("target")
	for _, v := range unrelated {
		v.reads.Store(0)
	}

	replaceBlueprintFieldReference(bp.NormalMember, from, to)
	after, _ := bp.NormalMember.Get("target")
	require.Equal(t, []Value{to, third, to}, after)
	require.Equal(t, []Value{from, third, from}, before, "reader snapshots must remain immutable")
	var reads int64
	for _, v := range unrelated {
		reads += v.reads.Load()
	}
	require.Zero(t, reads, "replacement must look up the affected fields, not scan unrelated members")

	// A subsequent rewrite follows the new identity, including both occurrences.
	replaceBlueprintFieldReference(bp.NormalMember, to, third)
	after, _ = bp.NormalMember.Get("target")
	require.Equal(t, []Value{third, third, third}, after)
	require.Zero(t, unrelated[0].reads.Load())
}

func TestBlueprintReplacementTracksLateIDsAndSet(t *testing.T) {
	bp := NewBlueprint("late")
	from, to := blueprintTestValue(-1), blueprintTestValue(2)
	bp.RegisterStaticMember("late", from, false)
	// An un-emitted value may acquire an ID after registration.
	from.SetId(1)
	replaceBlueprintFieldReference(bp.StaticMember, from, to)
	require.Equal(t, to, bp.GetStaticMember("late"))

	// Directly publishing a field must update the lookup too. Typed nils remain
	// nil; a value with the same ID but a different wrapper must be replaced.
	var nilConst *ConstInst
	alias := blueprintTestValue(1)
	bp.StaticMember.Set("direct", []Value{nilConst, alias, alias})
	replaceBlueprintFieldReference(bp.StaticMember, from, to)
	direct, _ := bp.StaticMember.Get("direct")
	require.Equal(t, []Value{nilConst, to, to}, direct)
	bp.StaticMember.Set("direct", []Value{from})
	replaceBlueprintFieldReference(bp.StaticMember, to, nil)
	direct, _ = bp.StaticMember.Get("direct")
	require.Equal(t, []Value{from}, direct, "stale lookup entries must not replace another value")
	replaceBlueprintFieldReference(bp.StaticMember, from, to)
	require.Equal(t, to, bp.GetStaticMember("direct"))
}

func TestBlueprintReplacementConcurrentAppendPreservesMembers(t *testing.T) {
	bp := NewBlueprint("concurrent")
	from, to := blueprintTestValue(1), blueprintTestValue(2)
	bp.RegisterNormalMember("shared", from, false)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bp.RegisterNormalMember("shared", blueprintTestValue(int64(100+i)), false)
		}(i)
	}
	replaceBlueprintFieldReference(bp.NormalMember, from, to)
	wg.Wait()
	values, _ := bp.NormalMember.Get("shared")
	require.Len(t, values, 17)
	require.Equal(t, to, values[0])
	seen := make(map[int64]bool)
	for _, v := range values[1:] {
		seen[v.GetId()] = true
	}
	require.Len(t, seen, 16)
}

func TestBlueprintReplacementMatchesHistoryWalk(t *testing.T) {
	bp := NewBlueprint("model")
	model := make(map[string][]Value)
	values := make([]Value, 32)
	for i := range values {
		values[i] = blueprintTestValue(int64(i + 1))
	}
	rng := rand.New(rand.NewSource(42))
	for step := 0; step < 500; step++ {
		name := fmt.Sprintf("f%d", rng.Intn(16))
		v := values[rng.Intn(len(values))]
		switch rng.Intn(3) {
		case 0:
			bp.RegisterNormalMember(name, v, false)
			old := model[name]
			if len(old) == 0 || old[len(old)-1] == nil || old[len(old)-1].GetId() != v.GetId() {
				model[name] = append(old, v)
			}
		case 1:
			bp.NormalMember.Set(name, []Value{v, nil, v})
			model[name] = []Value{v, nil, v}
		case 2:
			var to Value
			if rng.Intn(5) != 0 {
				to = values[rng.Intn(len(values))]
			}
			replaceBlueprintFieldReference(bp.NormalMember, v, to)
			for _, history := range model {
				for i, current := range history {
					if current != nil && current.GetId() == v.GetId() {
						history[i] = to
					}
				}
			}
		}
		for field, history := range model {
			got, ok := bp.NormalMember.Get(field)
			require.True(t, ok)
			require.Equal(t, history, got, "step %d field %s", step, field)
		}
	}
}

func TestBlueprintMemberIterationCanReenter(t *testing.T) {
	bp := NewBlueprint("reentrant")
	v := blueprintTestValue(1)
	bp.RegisterNormalMember("before", v, false)
	bp.NormalMember.ForEach(func(name string, values []Value) bool {
		require.Equal(t, "before", name)
		bp.RegisterNormalMember("after", values[0], false)
		return true
	})
	require.Equal(t, v, bp.GetNormalMember("after"))
}
