package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestNewNextNilIterReturnsNil(t *testing.T) {
	require.Nil(t, NewNext(nil, false))
}

func TestEmitNextNilIterReturnsNilTriple(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	key, field, ok := builder.EmitNext(nil, false)
	require.Nil(t, key)
	require.Nil(t, field)
	require.Nil(t, ok)
}
