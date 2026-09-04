package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestParameterTopDefsUsesOmittedDefault(t *testing.T) {
	build := func(t *testing.T, unresolvedExplicitArgument bool) Values {
		t.Helper()
		cfg, err := ssaconfig.New(
			ssaconfig.ModeSSACompile,
			ssaconfig.WithSetProgramName(t.Name()),
		)
		require.NoError(t, err)

		raw := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
		mainBuilder := raw.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))

		callee := mainBuilder.NewFunc("withDefault")
		calleeBuilder := mainBuilder.PushFunction(callee)
		param := calleeBuilder.NewParam("value")
		fallback := calleeBuilder.EmitConstInst("fallback")
		param.SetDefault(fallback)
		calleeBuilder.EmitReturn([]ssa.Value{param})
		calleeBuilder.Finish()

		call := mainBuilder.EmitCall(mainBuilder.NewCall(callee, nil))
		if unresolvedExplicitArgument {
			// Preserve the distinction between an omitted argument and an
			// explicitly supplied argument whose persisted id cannot resolve.
			call.Args = []int64{1 << 62}
		}
		mainBuilder.Finish()

		program := NewProgram(raw, nil)
		callValue, err := program.NewValue(call)
		require.NoError(t, err)
		return callValue.GetTopDefs()
	}

	t.Run("omitted argument", func(t *testing.T) {
		defs := build(t, false)
		require.Len(t, defs, 1)
		require.Equal(t, "fallback", defs[0].GetConstValue())
	})

	t.Run("unresolved explicit argument", func(t *testing.T) {
		defs := build(t, true)
		require.NotEmpty(t, defs)
		for _, def := range defs {
			require.NotEqual(t, "fallback", def.GetConstValue())
		}
	})
}
