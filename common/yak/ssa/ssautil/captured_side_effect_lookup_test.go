package ssautil

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var capturedLookupValue, capturedLookupBind VersionedIF[*dummySSAValue]

func TestCapturedSideEffectLookupPreservesBinding(t *testing.T) {
	scope := NewRootVersionedTable[*dummySSAValue]("lookup", NewVersioned[*dummySSAValue])
	first := scope.CreateVariable("effect", true)
	bind := scope.CreateVariable("bind", true)
	scope.SetCapturedSideEffect("effect", first, bind)
	got, gotBind := scope.ReadVariableFromLinkSideEffect("effect")
	require.Same(t, first, got)
	require.Same(t, bind, gotBind)

	replacement := scope.CreateVariable("replacement", true)
	scope.ChangeCapturedSideEffect("effect", replacement)
	got, gotBind = scope.ReadVariableFromLinkSideEffect("effect")
	require.Same(t, replacement, got)
	require.Same(t, bind, gotBind)

	got, gotBind = scope.ReadVariableFromLinkSideEffect("missing")
	require.Nil(t, got)
	require.Nil(t, gotBind)
}

func TestCapturedSideEffectLookupDoesNotSnapshotTable(t *testing.T) {
	scope := NewRootVersionedTable[*dummySSAValue]("lookup", NewVersioned[*dummySSAValue])
	for i := 0; i < 512; i++ {
		name := fmt.Sprintf("effect-%04d", i)
		variable := scope.CreateVariable(name, true)
		scope.SetCapturedSideEffect(name, variable, variable)
	}
	for _, name := range []string{"effect-0000", "effect-0511", "missing"} {
		allocs := testing.AllocsPerRun(20, func() {
			capturedLookupValue, capturedLookupBind = scope.ReadVariableFromLinkSideEffect(name)
		})
		require.Zero(t, allocs, "an exact lookup must not copy all keys and bindings for %s", name)
	}
}

func TestCapturedSideEffectLookupConcurrentPublication(t *testing.T) {
	scope := NewRootVersionedTable[*dummySSAValue]("lookup", NewVersioned[*dummySSAValue])
	variable := scope.CreateVariable("effect", true)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				scope.SetCapturedSideEffect("effect", variable, variable)
				got, bind := scope.ReadVariableFromLinkSideEffect("effect")
				if got != variable || bind != variable {
					t.Error("published value and binding must be read together")
					return
				}
			}
		}()
	}
	wg.Wait()
}
