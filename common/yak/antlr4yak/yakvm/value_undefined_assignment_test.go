package yakvm

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUndefinedAssignmentOwnsBindingMetadata(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "global"}[global], func(t *testing.T) {
			scope := NewScope(NewSymbolTable())
			first, second := NewValueRef(1), NewValueRef(2)
			if global {
				first.GlobalAssignBySymbol(scope, GetUndefined())
				second.GlobalAssignBySymbol(scope, NewAutoValue(nil))
			} else {
				first.AssignBySymbol(scope, GetUndefined())
				second.AssignBySymbol(scope, NewAutoValue(nil))
			}
			a, ok := scope.GetValueByID(1)
			require.True(t, ok)
			b, ok := scope.GetValueByID(2)
			require.True(t, ok)
			require.NotSame(t, a, b)
			require.NotSame(t, GetUndefined(), a)
			require.Same(t, first, a.CallerRef)
			require.Same(t, second, b.CallerRef)
			require.True(t, IsUndefined(a))
			require.True(t, a.IsUndefined())
			require.Nil(t, a.Value)
			require.Nil(t, GetUndefined().CallerRef)
		})
	}
}

func TestUndefinedAssignmentConcurrentScopes(t *testing.T) {
	var wg sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scope := NewScope(NewSymbolTable())
			for i := 1; i <= 64; i++ {
				left := NewValueRef(i)
				left.AssignBySymbol(scope, GetUndefined())
				left.GlobalAssignBySymbol(scope, NewAutoValue(nil))
			}
		}()
	}
	wg.Wait()
	require.Nil(t, GetUndefined().CallerRef)
}

func TestBoundUndefinedCallDiagnostics(t *testing.T) {
	scope := NewScope(NewSymbolTable())
	NewValueRef(1).AssignBySymbol(scope, GetUndefined())
	bound, ok := scope.GetValueByID(1)
	require.True(t, ok)
	frame := &Frame{}
	for _, value := range []*Value{GetUndefined(), bound} {
		require.PanicsWithValue(t, undefinedCallErrorMessage, func() { frame.call(value, false, nil) })
		require.PanicsWithValue(t, undefinedCallErrorMessage, func() { frame.asyncCall(value, false, nil) })
		require.PanicsWithValue(t, undefinedCallErrorMessage, func() { frame.callLua(value, nil) })
	}
	require.False(t, IsUndefined(nil))
	require.False(t, IsUndefined(NewValue("other", nil, "")))
}
