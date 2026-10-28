package ssa

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type testExternMethodBuilder struct {
	name  string
	calls atomic.Int64
}

func (b *testExternMethodBuilder) Build(Type, string) *Function {
	b.calls.Add(1)
	return nil
}

func (b *testExternMethodBuilder) GetMethodNames(Type) []string {
	return []string{b.name}
}

func TestExternMethodBuilderProgramIsolationAndFallback(t *testing.T) {
	previous := getExternMethodBuilder()
	t.Cleanup(func() { SetExternMethodBuilder(previous) })
	fallback := &testExternMethodBuilder{name: "fallback"}
	first := &testExternMethodBuilder{name: "first"}
	second := &testExternMethodBuilder{name: "second"}
	SetExternMethodBuilder(fallback)

	firstProgram := &Program{externMethodBuilder: first}
	secondProgram := &Program{externMethodBuilder: second}
	defaultProgram := &Program{}
	typ := NewBasicType(StringTypeKind, "string")
	require.Equal(t, []string{"first"}, firstProgram.GetAllKey(typ))
	require.Equal(t, []string{"second"}, secondProgram.GetAllKey(typ))
	require.Equal(t, []string{"fallback"}, defaultProgram.GetAllKey(typ))
	require.Equal(t, []string{"fallback"}, GetAllKey(typ))
	require.Equal(t, []string{"fallback"}, GetMethodsName(typ))
	firstProgram.getMethod(typ, "method")
	secondProgram.getMethod(typ, "method")
	defaultProgram.getMethod(typ, "method")
	GetMethod(typ, "method")
	require.EqualValues(t, 1, first.calls.Load())
	require.EqualValues(t, 1, second.calls.Load())
	require.EqualValues(t, 2, fallback.calls.Load())
}

func TestExternMethodBuilderFallbackConcurrentAccess(t *testing.T) {
	previous := getExternMethodBuilder()
	t.Cleanup(func() { SetExternMethodBuilder(previous) })
	first := &testExternMethodBuilder{name: "first"}
	second := &testExternMethodBuilder{name: "second"}
	SetExternMethodBuilder(first)
	var workers sync.WaitGroup
	workers.Add(5)
	go func() {
		defer workers.Done()
		for i := 0; i < 1000; i++ {
			SetExternMethodBuilder(first)
			SetExternMethodBuilder(second)
		}
	}()
	for i := 0; i < 4; i++ {
		go func() {
			defer workers.Done()
			typ := NewBasicType(StringTypeKind, "string")
			prog := &Program{}
			for j := 0; j < 1000; j++ {
				GetAllKey(typ)
				GetMethodsName(typ)
				GetMethod(typ, "method")
				prog.GetAllKey(typ)
				prog.getMethod(typ, "method")
			}
		}()
	}
	workers.Wait()
	require.EqualValues(t, 8000, first.calls.Load()+second.calls.Load())
}
