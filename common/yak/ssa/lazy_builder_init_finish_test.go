package ssa

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestProgramLazyBuildFinishesInitFunction(t *testing.T) {
	prog := NewProgram(context.Background(), t.Name(), ProgramCacheMemory, Application, filesys.NewVirtualFs(), "", 0)
	_ = prog.GetAndCreateFunctionBuilder("", string(InitFunctionName))

	initFunc := prog.GetFunction(string(InitFunctionName), "")
	require.NotNil(t, initFunc)
	require.Nil(t, initFunc.Type, "precondition: init function type should be nil before finish")

	prog.LazyBuild()

	require.NotNil(t, initFunc.Type, "init function should be finished by LazyBuild")
}
