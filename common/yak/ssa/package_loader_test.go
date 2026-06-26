package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newPackageLoaderTestProgram(t *testing.T) (*Program, *FunctionBuilder) {
	t.Helper()

	fs := filesys.NewVirtualFs()
	fs.AddFile("b.php", `<?php $x = 1;`)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName("package-loader-test"))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheMemory, Application, fs, "/tmp/project", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	editor := prog.CreateEditor([]byte(`<?php`), "/tmp/project/a.php")
	builder.SetEditor(editor)
	return prog, builder
}

func TestIncludeProgramInStack(t *testing.T) {
	prog, builder := newPackageLoaderTestProgram(t)
	subProg := NewTmpProgram("sub")

	require.False(t, builder.includeProgramInStack(nil))
	require.False(t, builder.includeProgramInStack(subProg))

	builder.includeStack.Push(subProg)
	require.True(t, builder.includeProgramInStack(subProg))
	require.False(t, builder.includeProgramInStack(prog))
}

func TestBuildFilePackageSkipsLazyBuildOnIncludeCycle(t *testing.T) {
	prog, builder := newPackageLoaderTestProgram(t)

	path, loadedEditor, err := prog.Loader.LoadFilePackage("b.php", false)
	require.NoError(t, err)

	cachedEditor := prog.CreateEditor([]byte(loadedEditor.GetSourceCode()), path)
	fileHash := cachedEditor.GetPureSourceHash()

	subProg := NewTmpProgram("cached-sub")
	subProg.Application = prog
	subBuilder := subProg.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	lazyRuns := 0
	subBuilder.AddLazyBuilder(func() {
		lazyRuns++
	}, true)
	prog.UpStream.Set(fileHash, subProg)

	builder.includeStack.Push(subProg)
	err = builder.BuildFilePackage("b.php", false)
	require.NoError(t, err)
	require.Equal(t, 0, lazyRuns, "LazyBuild must be skipped when sub program is already on include stack")
}
