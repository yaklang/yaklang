package ssadb_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestTypeData(t *testing.T) {
	t.Run("type data loss", func(t *testing.T) {
		programName := uuid.NewString()
		// code := `
		// package main

		// type T struct {
		// 	a int
		// 	b int
		// }
		// func main(){
		// 	o := &T{a: 1, b: 2}
		// 	f1 := func() {
		// 		if true {
		// 			o = &T{a: 3, b: 4}
		// 		}
		// 	}
		// 	f1()
		// 	c := o.a
		// 	d := o.b
		// }
		// `

		// prog, err := ssaapi.Parse(code,
		// 	ssaapi.WithLanguage(ssaapi.GO),
		// 	ssaapi.WithProgramName(programName))
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
		// require.NoError(t, err)

		prog := ssa.NewProgram(programName, true, ssa.Application, nil, "")
		prog.Language = "GO"

		prog.ShowWithSource()
		cache := prog.Cache

		builder := prog.GetAndCreateFunctionBuilder("", "main")
		builder.SetLanguageConfig(
			ssa.LanguageConfigIsBinding,
		)

		left := builder.CreateVariable("a")
		right := builder.InterfaceAddFieldBuild(3, func(i int) ssa.Value {
			return builder.EmitConstInst(i)
		}, func(i int) ssa.Value {
			return builder.EmitConstInst(i)
		})

		builder.AssignVariable(left, right)

		type1 := right.GetType()
		valueInMermory := right

		cache.SaveToDatabase()

		lazyInst := cache.GetInstruction(valueInMermory.GetId())
		require.NotNil(t, lazyInst)
		lz, isLazyInstruction := ssa.ToLazyInstruction(lazyInst)
		require.True(t, isLazyInstruction)
		lz.Self()
		type2 := lz.GetType()

		require.Equal(t, type1, type2)
	})
}
