package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func scopeLookupFixture() (*ScopeInstance, *Variable, *Variable) {
	root := NewScope(nil, "")
	parent := root.CreateVariable("x", true).(*Variable)
	root.AssignVariable(parent, NewConst(1))
	child := root.CreateSubScope().(*ScopeInstance)
	local := child.CreateVariable("x", true).(*Variable)
	child.AssignVariable(local, NewConst(2))
	for i := 0; i < 64; i++ {
		v := child.CreateVariable("x", false)
		child.AssignVariable(v, NewConst(i+3))
	}
	return child, local, parent
}

func TestScopeVariableLookupOrder(t *testing.T) {
	child, local, parent := scopeLookupFixture()
	all := GetAllVariablesFromScopeAndParent(child, "x")
	require.Len(t, all, 66)
	require.Same(t, all[0], GetFristVariableFromScope(child, "x"))
	require.Same(t, local, GetFristLocalVariableFromScope(child, "x"))
	require.Same(t, local, GetFristLocalVariableFromScopeAndParent(child, "x"))
	require.Same(t, parent, all[len(all)-1])
	empty := child.CreateSubScope().(*ScopeInstance)
	require.Nil(t, GetFristLocalVariableFromScope(empty, "x"))
	require.Same(t, local, GetFristLocalVariableFromScopeAndParent(empty, "x"))
	// Snapshots remain independent and must not change after later assignments.
	snapshot := child.GetAllVariablesByName("x", true)
	snapshot[0] = nil
	require.NotNil(t, GetFristVariableFromScope(child, "x"))
	newVar := child.CreateVariable("x", true)
	child.AssignVariable(newVar, NewConst(99))
	require.Len(t, snapshot, 65)
	require.Same(t, newVar, GetFristVariableFromScope(child, "x"))
}

var scopeLookupResult *Variable

func TestScopeVariableLookupAllocations(t *testing.T) {
	child, _, _ := scopeLookupFixture()
	allocs := testing.AllocsPerRun(100, func() {
		scopeLookupResult = GetFristLocalVariableFromScopeAndParent(child, "x")
	})
	require.Zero(t, allocs, "finding one version must not snapshot the chain")
}

func BenchmarkScopeVariableLookup(b *testing.B) {
	child, _, _ := scopeLookupFixture()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scopeLookupResult = GetFristLocalVariableFromScopeAndParent(child, "x")
	}
}
