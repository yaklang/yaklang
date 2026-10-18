package ssa

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// GlobalVariablesBlueprint.StaticMember is one map shared by parallel compile
// units. RegisterStaticMember appends while LoadGlobalVariable and other units
// read it. Both sides must stay race-free, and every registered value must
// still be readable afterwards.
func TestBlueprintStaticMemberConcurrentRegisterAndRead(t *testing.T) {
	bp := NewBlueprint("globals")
	builder := testBuilderNamed(t, t.Name())
	const n = 32
	shared := make([]Value, n)
	unique := make([]Value, n)
	for i := 0; i < n; i++ {
		shared[i] = builder.EmitConstInst(i)
		unique[i] = builder.EmitConstInst(i + 1000)
		require.NotZero(t, shared[i].GetId())
		require.NotZero(t, unique[i].GetId())
	}

	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			bp.RegisterStaticMember(fmt.Sprintf("u%d", i), unique[i], false)
			bp.RegisterStaticMember("shared", shared[i], false)
			bp.RegisterNormalMember(fmt.Sprintf("n%d", i), unique[i], false)
			bp.RegisterConstMember(fmt.Sprintf("c%d", i), unique[i], false)
		}(i)
		go func() {
			defer wg.Done()
			_ = bp.GetStaticMember("shared")
			_ = bp.GetStaticMembers("shared")
			_ = bp.GetNormalMember("n0")
			_ = bp.GetConstMember("c0")
			for _, name := range bp.StaticMember.Keys() {
				if name == "" {
					continue
				}
				_ = bp.GetStaticMember(name)
			}
		}()
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		got := bp.GetStaticMember(fmt.Sprintf("u%d", i))
		require.False(t, got == nil)
		require.Equal(t, unique[i].GetId(), got.GetId())
		got = bp.GetNormalMember(fmt.Sprintf("n%d", i))
		require.Equal(t, unique[i].GetId(), got.GetId())
		got = bp.GetConstMember(fmt.Sprintf("c%d", i))
		require.Equal(t, unique[i].GetId(), got.GetId())
	}
	seen := map[int64]int{}
	for _, member := range bp.GetStaticMembers("shared") {
		seen[member.GetId()]++
	}
	require.Len(t, seen, n)
	for i := 0; i < n; i++ {
		require.Equal(t, 1, seen[shared[i].GetId()])
	}
}
