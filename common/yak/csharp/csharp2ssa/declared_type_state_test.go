package csharp2ssa

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestDeclaredTypeStateResetsWithProjectPartition(t *testing.T) {
	builder := CreateBuilder().(*SSABuilder)
	first := filesys.NewVirtualFs()
	first.AddFile("First.cs", `class First { }`)
	builder.PartitionCompileUnits(first, []string{"First.cs"})

	owner := ssa.NewBlueprint("First")
	slot := csharpDeclaredMemberSlot{owner: owner, name: "Value"}
	value := ssa.NewConst("first-project")
	builder.declaredTypes.Lock()
	builder.declaredTypes.members = map[csharpDeclaredMemberSlot]ssa.Type{
		slot: ssa.CreateStringType(),
	}
	builder.declaredTypes.writers = map[csharpDeclaredValueKey]csharpDeclaredMemberSlot{
		{program: value.GetProgram(), id: value.GetId()}: slot,
	}
	builder.declaredTypes.Unlock()

	second := filesys.NewVirtualFs()
	second.AddFile("Second.cs", `class Second { }`)
	builder.PartitionCompileUnits(second, []string{"Second.cs"})

	builder.declaredTypes.RLock()
	defer builder.declaredTypes.RUnlock()
	require.Nil(t, builder.declaredTypes.members)
	require.Nil(t, builder.declaredTypes.writers)
}

func TestDeclaredTypeStateResetIsConcurrentSafe(t *testing.T) {
	var state csharpDeclaredTypeState
	builder := &singleFileBuilder{declaredTypes: &state}
	owner := ssa.NewBlueprint("Concurrent")
	typ := ssa.CreateStringType()

	const workers = 8
	const iterations = 100
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			value := ssa.NewConst(fmt.Sprintf("writer%d", worker))
			receiver := ssa.NewConst(fmt.Sprintf("receiver%d", worker))
			receiver.SetType(owner)
			for iteration := 0; iteration < iterations; iteration++ {
				name := fmt.Sprintf("Value%d", worker)
				slot := csharpDeclaredMemberSlot{owner: owner, name: name}
				builder.registerDeclaredMemberType(owner, name, false, typ)
				_, _, _ = builder.declaredMemberSlotForReceiver(receiver, name)
				builder.rememberDeclaredMemberWriter(value, slot)
				_, _ = builder.declaredMemberWriter(value)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			for iteration := 0; iteration < iterations; iteration++ {
				state.reset()
			}
		}()
	}
	close(start)
	wg.Wait()
	state.reset()

	state.RLock()
	defer state.RUnlock()
	require.Nil(t, state.members)
	require.Nil(t, state.writers)
}
