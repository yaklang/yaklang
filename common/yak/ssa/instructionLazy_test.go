package ssa_test

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// add test for lazyinstruction init in multiple goroutine
func TestLazyInstruction(t *testing.T) {
	progName := uuid.NewString()
	db := consts.GetGormDefaultSSADataBase()

	prog := ssa.NewProgram(progName, ssa.ProgramCacheDBRead, ssa.Application, nil, "", 0)
	require.NotNil(t, prog)
	saveIrCode := func(full func(*ssadb.IrCode, map[string]any)) int64 {
		id, ir := ssadb.RequireIrCode(db, progName)
		params := make(map[string]any)
		full(ir, params)
		ir.SetExtraInfo(params)
		ir.Save(db)
		return id
	}

	method := saveIrCode(func(ic *ssadb.IrCode, params map[string]any) {
		ic.Opcode = int64(ssa.SSAOpcodeUndefined)
		ic.VerboseName = "methodName"

	})

	call := saveIrCode(func(ic *ssadb.IrCode, params map[string]any) {
		ic.Opcode = int64(ssa.SSAOpcodeCall)
		params["call_method"] = method
	})

	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst, err := ssa.NewLazyInstruction(prog, call)
			require.NoError(t, err)
			require.NotNil(t, inst)
			require.Equal(t, ssa.SSAOpcodeCall, inst.GetOpcode())
			callInst, ok := ssa.ToCall(inst)
			require.True(t, ok)
			require.Equal(t, method, callInst.Method)
		}()
	}

}
