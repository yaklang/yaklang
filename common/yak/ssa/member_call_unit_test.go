package ssa

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestMissingDependencyMembersStayInConsumerUnit(t *testing.T) {
	prog, dependency := newTestBuilder(t)
	object := emitObject(dependency, "shared")
	original := append([]int64(nil), dependency.CurrentBlock.Insts...)
	const count = 16
	builders := make([]*FunctionBuilder, count)
	keys := make([]Value, count)
	members := make([]Value, count)
	for i := range builders {
		lib := prog.NewLibrary(fmt.Sprintf("consumer%d", i), nil)
		builders[i] = lib.GetAndCreateFunctionBuilder("", string(MainFunctionName))
		keys[i] = builders[i].EmitConstInstPlaceholder(fmt.Sprintf("missing%d", i))
	}
	var wg sync.WaitGroup
	for i := range builders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			members[i] = builders[i].ReadMemberCallMethod(object, keys[i])
		}(i)
	}
	wg.Wait()
	require.Equal(t, original, dependency.CurrentBlock.Insts, "consumers must not rewrite the dependency CFG")
	for i, member := range members {
		require.NotNil(t, member)
		require.Same(t, builders[i].Function, member.GetFunc())
		linked, ok := GetLatestMemberByKey(object, keys[i])
		require.True(t, ok)
		require.Equal(t, member.GetId(), linked.GetId(), "cross-unit member relations must remain visible")
	}
}
